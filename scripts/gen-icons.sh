#!/usr/bin/env bash
# Renders every app icon and favicon from the brand mark (design-system/lottery-platform/logo). Run after changing the
# mark; the PNGs are committed. Needs rsvg-convert (librsvg).
set -euo pipefail
cd "$(dirname "$0")/.."
LOGO=design-system/lottery-platform/logo
MARK=$LOGO/4draws-mark.svg          # rounded tile: web icons (192 and up), Android legacy launcher icons
FAV=$LOGO/4draws-favicon.svg        # the 4 alone, bold: browser tabs and anything under 64 px, where the ring would blur
SQUARE=$LOGO/4draws-mark-square.svg # full-bleed: iOS (rounds its own corners), maskable web icons
png() { rsvg-convert -w "$2" -h "$2" "$1" -o "$3"; }

# the share image (Open Graph / Twitter card, 1200x630) for the web app's links
rsvg-convert -w 1200 -h 630 "$LOGO/4draws-og.svg" -o apps/mobile/web/og-image.png

# an .ico holding 16, 32 and 48 px, each rendered from the vector at its own size
ico() {
  local tmp; tmp=$(mktemp -d)
  for s in 16 32 48; do png "$FAV" "$s" "$tmp/$s.png"; done
  magick "$tmp/16.png" "$tmp/32.png" "$tmp/48.png" "$1"
  rm -rf "$tmp"
}

# Player web app (Flutter web)
W=apps/mobile/web
png "$FAV" 48 "$W/favicon.png"
cp "$FAV" "$W/favicon.svg"
ico "$W/favicon.ico"
png "$MARK" 192 "$W/icons/Icon-192.png"
png "$MARK" 512 "$W/icons/Icon-512.png"
png "$SQUARE" 192 "$W/icons/Icon-maskable-192.png"
png "$SQUARE" 512 "$W/icons/Icon-maskable-512.png"

# Back office (Vite serves public/ at the site root)
B=apps/backoffice/public
mkdir -p "$B"
cp "$FAV" "$B/favicon.svg"
png "$FAV" 48 "$B/favicon.png"
ico "$B/favicon.ico"
png "$SQUARE" 180 "$B/apple-touch-icon.png"

# Android: legacy launcher PNGs per density (the adaptive icon is the vector drawable next to them)
A=apps/mobile/android/app/src/main/res
for d in mdpi:48 hdpi:72 xhdpi:96 xxhdpi:144 xxxhdpi:192; do
  png "$MARK" "${d#*:}" "$A/mipmap-${d%%:*}/ic_launcher.png"
done

# iOS: every size in the asset catalogue, full-bleed and opaque
I=apps/mobile/ios/Runner/Assets.xcassets/AppIcon.appiconset
python3 - "$I" <<'PY'
import json, subprocess, sys
d = sys.argv[1]
for img in json.load(open(f"{d}/Contents.json"))["images"]:
    pt = float(img["size"].split("x")[0]); scale = int(img["scale"].rstrip("x"))
    px = int(round(pt * scale))
    subprocess.run(["rsvg-convert", "-w", str(px), "-h", str(px), "-b", "#4338CA",
                    "design-system/lottery-platform/logo/4draws-mark-square.svg", "-o", f"{d}/{img['filename']}"], check=True)
PY
echo "icons rendered"
