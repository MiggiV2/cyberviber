package serial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
)

// The device answers {"type":"screenshot","y":0,"rows":320} with its screen in strips,
// {"type":"shot","w":240,"h":320,"y":0,"n":4,"data":"<base64 RGB565, little-endian>"},
// followed by {"type":"shot_end"}. Strips that arrive damaged are requested again.
type shotLine struct {
	W, H, Y, N int
	Data       []byte // base64 in JSON
}

const shotRounds = 4

type shot struct {
	img     *image.RGBA
	got     map[int]int // y → rows received
	strip   int
	pending int // shot_end markers still expected
	rounds  int
	done    chan struct{}
	err     error
}

// Screenshot asks the device for its screen. It takes ~20 s at 115200 baud.
func (l *Link) Screenshot(ctx context.Context) (image.Image, error) {
	s := &shot{done: make(chan struct{}), got: map[int]int{}, pending: 1, rounds: 1}
	l.mu.Lock()
	if l.shot != nil {
		l.mu.Unlock()
		return nil, errors.New("screenshot already in progress")
	}
	l.shot = s
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		if l.shot == s {
			l.shot = nil
		}
		l.mu.Unlock()
		l.Notify() // session pushes were held back meanwhile
	}()
	if err := l.Send([]byte(`{"type":"screenshot"}`)); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return s.img, s.err
	}
}

func (l *Link) shotBusy() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.shot != nil
}

// shotLine stores one strip; damaged strips are skipped and fetched again after shot_end.
func (l *Link) shotLine(line []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.shot
	if s == nil {
		return
	}
	var m shotLine
	if err := json.Unmarshal(line, &m); err != nil {
		return
	}
	if m.W <= 0 || m.H <= 0 || m.W > 1024 || m.H > 1024 || m.Y < 0 || m.N <= 0 || m.Y+m.N > m.H || len(m.Data) != m.W*m.N*2 {
		return
	}
	if s.img == nil {
		s.img = image.NewRGBA(image.Rect(0, 0, m.W, m.H))
		s.strip = m.N
	}
	if s.img.Rect.Dx() != m.W || s.img.Rect.Dy() != m.H {
		return
	}
	for i := 0; i < m.W*m.N; i++ {
		v := uint16(m.Data[2*i]) | uint16(m.Data[2*i+1])<<8
		r, g, b := uint8(v>>11), uint8(v>>5&0x3f), uint8(v&0x1f)
		s.img.SetRGBA(i%m.W, m.Y+i/m.W, color.RGBA{r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2, 0xff})
	}
	s.got[m.Y] = m.N
}

// shotEnd finishes the screenshot or asks again for the strips that are missing.
func (l *Link) shotEnd(send func([]byte)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.shot
	if s == nil {
		return
	}
	if s.pending--; s.pending > 0 {
		return
	}
	finish := func(err error) {
		s.err = err
		close(s.done)
		l.shot = nil
	}
	var missing []int
	if s.img == nil {
		missing = []int{-1} // nothing usable yet: everything again
	} else {
		for y := 0; y < s.img.Rect.Dy(); {
			if n := s.got[y]; n > 0 {
				y += n
				continue
			}
			missing = append(missing, y)
			y += s.strip
		}
	}
	if len(missing) == 0 {
		finish(nil)
		return
	}
	if s.rounds >= shotRounds {
		finish(fmt.Errorf("screenshot: %d strips still damaged after %d rounds", len(missing), s.rounds))
		return
	}
	s.rounds++
	l.Logf("serial: screenshot: requesting %d damaged strips again", len(missing))
	s.pending = len(missing)
	for _, y := range missing {
		if y < 0 {
			send([]byte(`{"type":"screenshot"}`))
		} else {
			send([]byte(fmt.Sprintf(`{"type":"screenshot","y":%d,"rows":%d}`, y, s.strip)))
		}
	}
}
