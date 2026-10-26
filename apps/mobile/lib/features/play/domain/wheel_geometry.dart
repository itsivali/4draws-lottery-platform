import 'dart:convert';
import 'dart:math';

const double _tau = 2 * pi;

/// Maps [angle] (radians) into [0, 2π).
double normalizeAngle(double angle) {
  final r = angle % _tau; // Dart's % is never negative for a positive divisor
  return r >= _tau ? 0 : r;
}

/// The wheel's angles, in one place so no widget carries its own offsets.
///
/// Conventions: angles are radians, clockwise (Flutter's canvas has y pointing down), measured in the wheel's own frame
/// from the fixed pointer at 12 o'clock. Segment `i` covers `[i·sweep, (i+1)·sweep)` and shows slot `i` of the
/// server's spin. Rotating the wheel by `r` moves wheel angle `a` to screen angle `a + r`, so the pointer reads wheel
/// angle `-r`.
///
/// The geometry depends only on the slot count: the painter draws a unit wheel scaled to its size, so resizing never
/// changes which slot is under the pointer.
class WheelGeometry {
  WheelGeometry({required this.slots, this.luckySlots = 0}) {
    if (slots < 2) throw ArgumentError.value(slots, 'slots', 'a wheel needs at least two slots');
    if (luckySlots < 0 || luckySlots > slots) {
      throw ArgumentError.value(luckySlots, 'luckySlots', 'must be between 0 and slots');
    }
  }

  final int slots;

  /// Slots `0 … luckySlots-1` are lucky (the API's definition).
  final int luckySlots;

  double get sweep => _tau / slots;

  double segmentStart(int i) => i * sweep;

  double segmentCenter(int i) => (i + 0.5) * sweep;

  bool isLucky(int i) => i < luckySlots;

  /// The segment under the pointer when the wheel is turned by [rotation].
  int segmentAtPointer(double rotation) {
    final i = (normalizeAngle(-rotation) / sweep).floor();
    return i.clamp(0, slots - 1);
  }

  /// The rotation in [0, 2π) that rests [segment] under the pointer, [within] of the way through it (0.5 = centre).
  double restingRotationFor(int segment, {double within = 0.5}) {
    RangeError.checkValueInInterval(segment, 0, slots - 1, 'segment');
    return normalizeAngle(-(segment + within) * sweep);
  }

  /// The smallest rotation at least [minTravel] past [from] that rests [segment] under the pointer.
  double targetRotation({required double from, required int segment, double minTravel = 0, double within = 0.5}) {
    final base = restingRotationFor(segment, within: within);
    final turns = ((from + minTravel - base) / _tau).ceil();
    return base + turns * _tau;
  }

  /// Rotations of the form `base + k·2π` that rest [segment] under the pointer, the one whose travel from [from] is
  /// closest to [preferredTravel] without falling below [minTravel].
  double targetNear({
    required double from,
    required int segment,
    required double preferredTravel,
    required double minTravel,
    double within = 0.5,
  }) {
    final lowest = targetRotation(from: from, segment: segment, minTravel: minTravel, within: within);
    final extraTurns = ((from + preferredTravel - lowest) / _tau).round();
    return lowest + max(0, extraTurns) * _tau;
  }

  /// How many segment boundaries pass the pointer as the wheel turns from [from] to [to].
  int boundariesCrossed(double from, double to) => ((to / sweep).floor() - (from / sweep).floor()).abs();

  @override
  bool operator ==(Object other) => other is WheelGeometry && other.slots == slots && other.luckySlots == luckySlots;

  @override
  int get hashCode => Object.hash(slots, luckySlots);
}

/// Where inside its segment a ticket's spin comes to rest: a fixed fraction in [0.25, 0.75] derived from the ticket
/// id, so the stop looks natural, is the same every time the receipt is replayed and never sits on an edge. It is
/// presentation only; the slot itself always comes from the server.
double landingFraction(String ticketId) {
  var h = 0x811c9dc5; // FNV-1a, 32 bit
  for (final b in utf8.encode(ticketId)) {
    h ^= b;
    h = (h * 0x01000193) & 0xffffffff;
  }
  return 0.25 + 0.5 * (h / 0xffffffff);
}
