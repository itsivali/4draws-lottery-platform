#!/usr/bin/env bash
# Renders every app icon and favicon from the brand mark (design-system/lottery-platform/logo). Run after changing the
# mark; the PNGs are committed. Needs rsvg-convert (librsvg).
set -euo pipefail
cd "$(dirname "$0")/.."
LOGO=design-system/lottery-platform/logo
MARK=$LOGO/tajirika-mark.svg          # rounded tile: favicons, web icons, Android legacy launcher icons
SQUARE=$LOGO/tajirika-mark-square.svg # full-bleed: iOS (rounds its own corners), maskable web icons
png() { rsvg-convert -w "$2" -h "$2" "$1" -o "$3"; }

# Player web app (Flutter web)
W=apps/mobile/web
png "$MARK" 48 "$W/favicon.png"
cp "$MARK" "$W/favicon.svg"
png "$MARK" 192 "$W/icons/Icon-192.png"
png "$MARK" 512 "$W/icons/Icon-512.png"
png "$SQUARE" 192 "$W/icons/Icon-maskable-192.png"
png "$SQUARE" 512 "$W/icons/Icon-maskable-512.png"

# Back office (Vite serves public/ at the site root)
B=apps/backoffice/public
mkdir -p "$B"
cp "$MARK" "$B/favicon.svg"
png "$MARK" 32 "$B/favicon-32.png"
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
    subprocess.run(["rsvg-convert", "-w", str(px), "-h", str(px), "-b", "#C23412",
                    "design-system/lottery-platform/logo/tajirika-mark-square.svg", "-o", f"{d}/{img['filename']}"], check=True)
PY
echo "icons rendered"
