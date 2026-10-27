#!/usr/bin/env bash
# Runs the player app on Android against the local backend (scripts/dev-backend.sh), with hot reload (press r).
# Uses a phone connected over USB if there is one (reaching the backend through adb reverse), otherwise an emulator,
# starting it (and creating it, the first time) when needed.
#
#   scripts/dev-mobile.sh                 run on the connected phone or the emulator
#   scripts/dev-mobile.sh --emulator      always use the emulator
#
# Needs: mise install (pinned Flutter and JDK 17), and an Android SDK with platform-tools, platforms;android-36,
# build-tools;36.0.0, emulator and system-images;android-36;google_apis;x86_64 (ANDROID_HOME, default ~/Android/Sdk).
set -euo pipefail
root=$(git rev-parse --show-toplevel)
API_PORT=8080
AVD=${AVD:-lottery_pixel}
export ANDROID_HOME=${ANDROID_HOME:-$HOME/Android/Sdk}
# avdmanager follows XDG_CONFIG_HOME, the emulator looks in ~/.android unless told otherwise
if [ -z "${ANDROID_AVD_HOME:-}" ]; then
  for d in "$HOME/.android/avd" "${XDG_CONFIG_HOME:-$HOME/.config}/.android/avd"; do
    [ -f "$d/$AVD.ini" ] && export ANDROID_AVD_HOME=$d
  done
fi

adb="$ANDROID_HOME/platform-tools/adb"
[ -x "$adb" ] || { echo "No Android SDK at $ANDROID_HOME (set ANDROID_HOME)" >&2; exit 1; }
command -v mise >/dev/null || { echo "mise is required (https://mise.jdx.dev): then run mise install" >&2; exit 1; }
curl -fsS "http://localhost:$API_PORT/readyz" >/dev/null 2>&1 \
  || { echo "The backend is not running: start it with scripts/dev-backend.sh" >&2; exit 1; }
JAVA_HOME=$(cd "$root" && mise where java)
export JAVA_HOME

phone=$("$adb" devices | awk 'NR>1 && $2=="device" && $1 !~ /^emulator-/ {print $1; exit}')
if [ -n "$phone" ] && [ "${1:-}" != --emulator ]; then
  device=$phone
  "$adb" -s "$device" reverse "tcp:$API_PORT" "tcp:$API_PORT" >/dev/null # the phone's localhost:8080 is this machine's
  api="http://localhost:$API_PORT"
  echo "Using the phone $device"
else
  device=$("$adb" devices | awk 'NR>1 && $2=="device" && $1 ~ /^emulator-/ {print $1; exit}')
  if [ -z "$device" ]; then
    if ! "$ANDROID_HOME/emulator/emulator" -list-avds | grep -qx "$AVD"; then
      echo "Creating the emulator $AVD (Pixel 7, Android 16)..."
      echo no | "$ANDROID_HOME/cmdline-tools/latest/bin/avdmanager" create avd -n "$AVD" \
        -k "system-images;android-36;google_apis;x86_64" -d pixel_7 >/dev/null
      export ANDROID_AVD_HOME=${ANDROID_AVD_HOME:-$HOME/.android/avd}
      [ -f "$ANDROID_AVD_HOME/$AVD.ini" ] || export ANDROID_AVD_HOME="${XDG_CONFIG_HOME:-$HOME/.config}/.android/avd"
    fi
    echo "Starting the emulator $AVD..."
    # in its own session, so it keeps running after this script (and Ctrl-C) ends
    setsid nohup "$ANDROID_HOME/emulator/emulator" -avd "$AVD" -no-snapshot-save -no-boot-anim -memory 2048 \
      >"${TMPDIR:-/tmp}/lottery-emulator.log" 2>&1 &
    "$adb" wait-for-device
    until [ "$("$adb" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ]; do sleep 2; done
    device=$("$adb" devices | awk 'NR>1 && $2=="device" && $1 ~ /^emulator-/ {print $1; exit}')
  fi
  api="http://10.0.2.2:$API_PORT" # the emulator's address for this machine
  echo "Using the emulator $device"
fi

cd "$root/apps/mobile"
echo "Building and installing (the first Gradle build takes several minutes)..."
exec mise exec -- flutter run -d "$device" --dart-define=API_BASE_URL="$api"
