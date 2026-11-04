#!/bin/sh
# Regenerates src/fonts/*.c: JetBrains Mono for text, DejaVu Sans for the spinner dingbats
# (✢✳✻✽) that JetBrains Mono lacks. Needs curl, unzip and node (npx lv_font_conv).
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -sSL -o "$tmp/jbm.zip" https://github.com/JetBrains/JetBrainsMono/releases/download/v2.304/JetBrainsMono-2.304.zip
curl -sSL -o "$tmp/dv.zip" https://github.com/dejavu-fonts/dejavu-fonts/releases/download/version_2_37/dejavu-fonts-ttf-2.37.zip
unzip -oq "$tmp/jbm.zip" -d "$tmp/jbm"
unzip -oq "$tmp/dv.zip" -d "$tmp/dv"
REG="$tmp/jbm/fonts/ttf/JetBrainsMono-Regular.ttf"
BOLD="$tmp/jbm/fonts/ttf/JetBrainsMono-Bold.ttf"
DV="$tmp/dv/dejavu-fonts-ttf-2.37/ttf/DejaVuSans-Bold.ttf"

# text: ASCII, Latin-1, punctuation and the state/arrow symbols
TEXT=0x20-0x7E,0xA0-0xFF,0x2013,0x2014,0x2018,0x2019,0x201C,0x201D,0x2022,0x2026,0x203A,0x2190,0x2192,0x2193,0x25CF,0x25E6,0x25C9,0x2713,0x2717,0x2736,0x258F,0x27F6
DING=0x2722,0x2733,0x273B,0x273D

gen() { # name size font
  npx -y lv_font_conv@1.5.3 --bpp 4 --size "$2" --no-compress --format lvgl --lv-include lvgl.h \
    --font "$3" -r "$TEXT" --font "$DV" -r "$DING" -o "src/fonts/$1.c"
}
gen cv_11 11 "$REG"
gen cv_11b 11 "$BOLD"
gen cv_13b 13 "$BOLD"
gen cv_14b 14 "$BOLD"
gen cv_22b 22 "$BOLD"
# hyprlock clock: digits and colon only
npx -y lv_font_conv@1.5.3 --bpp 4 --size 64 --no-compress --format lvgl --lv-include lvgl.h \
  --font "$BOLD" -r 0x30-0x3A -o src/fonts/cv_64b.c
npx -y lv_font_conv@1.5.3 --bpp 4 --size 40 --no-compress --format lvgl --lv-include lvgl.h \
  --font "$BOLD" -r 0x21,0xB7,0x2713,0x25E6,0x2736 --font "$DV" -r "$DING" -o src/fonts/cv_40.c
# keep the headers reproducible: drop the temp directory from the recorded options
sed -i '' 's#--font [^ ]*/\([^/ ]*\.ttf\)#--font \1#g' src/fonts/*.c
echo "fonts written to src/fonts/"
