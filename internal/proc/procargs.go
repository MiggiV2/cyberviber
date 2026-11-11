package proc

import "bytes"

// parseProcargs splits a kern.procargs2 buffer:
// int32 argc, exec path, NUL padding, argc × argv, env strings, empty string.
func parseProcargs(b []byte) (exe string, argv, env []string) {
	if len(b) < 4 {
		return "", nil, nil
	}
	argc := int(int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16 | int32(b[3])<<24)
	rest := b[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return "", nil, nil
	}
	exe, rest = string(rest[:i]), bytes.TrimLeft(rest[i:], "\x00")
	for len(rest) > 0 {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			j = len(rest)
		}
		str := string(rest[:j])
		if j < len(rest) {
			rest = rest[j+1:]
		} else {
			rest = nil
		}
		if len(argv) < argc {
			argv = append(argv, str)
			continue
		}
		if str == "" {
			break
		}
		env = append(env, str)
	}
	return exe, argv, env
}
