import '../../../core/platform/haptics.dart';
import '../domain/wheel_geometry.dart';
import '../domain/wheel_motion.dart';

/// Turns the wheel's actual motion into restrained tactile cues. It is fed every frame with the rotation the wheel is
/// drawn at, so ticks are tied to real pointer crossings and can never drift from the picture (no timers).
///
/// - one `start` per spin, however often the play button is pressed;
/// - `tick`s only while slowing, only when the wheel is slow enough that each tick maps to a visible segment, at
///   least [minGap] apart and at most [maxTicks] per spin;
/// - one `stop` per spin, and only when the wheel stopped on a server outcome.
class SpinHaptics {
  SpinHaptics(this._haptics);

  static const maxTicks = 28;
  static const minGap = Duration(milliseconds: 45);

  /// Above this many segments a second the ticks would blur into a buzz.
  static const maxSegmentsPerSecond = 14.0;

  final Haptics _haptics;

  String? _spinId;
  bool _settled = false;
  double? _lastRotation;
  Duration? _lastTick;
  int _ticks = 0;

  void begin(String spinId) {
    if (spinId == _spinId) return;
    _spinId = spinId;
    _settled = false;
    _lastRotation = null;
    _lastTick = null;
    _ticks = 0;
    _haptics.play(HapticEvent.start);
  }

  void onFrame({
    required Duration at,
    required WheelGeometry geometry,
    required double rotation,
    required double velocity,
    required MotionPhase phase,
  }) {
    final previous = _lastRotation;
    _lastRotation = rotation;
    if (_spinId == null || _settled || previous == null || phase != MotionPhase.decelerating) return;
    if (geometry.boundariesCrossed(previous, rotation) == 0) return;
    if (velocity / geometry.sweep > maxSegmentsPerSecond) return;
    if (_ticks >= maxTicks) return;
    if (_lastTick != null && at - _lastTick! < minGap) return;
    _lastTick = at;
    _ticks++;
    _haptics.play(HapticEvent.tick);
  }

  void onSettled({required bool hasOutcome}) {
    if (_spinId == null || _settled) return;
    _settled = true;
    if (hasOutcome) _haptics.play(HapticEvent.stop);
  }
}
