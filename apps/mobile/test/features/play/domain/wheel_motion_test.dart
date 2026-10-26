import 'dart:math';

import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/features/play/domain/wheel_geometry.dart';
import 'package:lottery_mobile/features/play/domain/wheel_motion.dart';

Duration ms(num v) => Duration(microseconds: (v * 1000).round());

void main() {
  final g = WheelGeometry(slots: 12, luckySlots: 1);
  const timing = SpinTiming();

  double numericVelocity(WheelMotion m, Duration t) {
    const h = Duration(microseconds: 500);
    return (m.rotationAt(t + h) - m.rotationAt(t - h)) / (2 * h.inMicroseconds / 1e6);
  }

  test('starts from rest and ramps smoothly to the cruising speed', () {
    final m = WheelMotion.start(rotation: 1.0, at: Duration.zero, timing: timing);
    expect(m.rotationAt(Duration.zero), 1.0);
    expect(m.velocityAt(Duration.zero), 0);
    expect(m.phaseAt(ms(100)), MotionPhase.accelerating);
    expect(m.velocityAt(timing.accelerate), closeTo(timing.peakVelocity, 1e-9));
    for (final t in [50, 300, 599, 601, 900]) {
      expect(m.velocityAt(ms(t)), closeTo(numericVelocity(m, ms(t)), 1e-3), reason: 'velocity matches motion at $t ms');
    }
  });

  test('keeps turning at a steady speed while the result is awaited, however long that takes', () {
    final m = WheelMotion.start(rotation: 0, at: Duration.zero, timing: timing);
    expect(m.phaseAt(ms(20000)), MotionPhase.cruising);
    expect(m.velocityAt(ms(20000)), closeTo(timing.peakVelocity, 1e-9));
    expect(m.settledAt(ms(20000)), isFalse);
    expect(m.target, isNull);
  });

  for (var slot = 0; slot < 12; slot++) {
    test('lands exactly on slot $slot and stops there', () {
      final m = WheelMotion.start(rotation: 0.3, at: Duration.zero, timing: timing);
      m.land(at: ms(400), geometry: g, slot: slot, within: 0.5);
      final end = m.settlesAt!;
      expect(m.settledAt(end), isTrue);
      expect(g.segmentAtPointer(m.rotationAt(end)), slot);
      expect(m.rotationAt(end + const Duration(seconds: 5)), m.rotationAt(end));
      expect(m.velocityAt(end), closeTo(0, 1e-9));
    });
  }

  test('a quick answer still gives a full spin of about four to six seconds', () {
    for (var slot = 0; slot < 12; slot++) {
      final m = WheelMotion.start(rotation: 0, at: Duration.zero, timing: timing);
      m.land(at: ms(150), geometry: g, slot: slot, within: landingFraction('t$slot'));
      expect(m.settlesAt!.inMilliseconds, inInclusiveRange(4000, 6000), reason: 'slot $slot');
    }
  });

  test('a late answer starts slowing when it arrives, never jumping', () {
    final m = WheelMotion.start(rotation: 0, at: Duration.zero, timing: timing);
    m.land(at: ms(7000), geometry: g, slot: 4, within: 0.5);
    expect(m.phaseAt(ms(6999)), MotionPhase.cruising);
    expect(m.phaseAt(ms(7001)), MotionPhase.decelerating);
    final before = m.rotationAt(ms(6999));
    final after = m.rotationAt(ms(7001));
    expect(after - before, closeTo(timing.peakVelocity * 0.002, 1e-3), reason: 'no jump at the hand-off');
    expect(m.velocityAt(ms(7001)), closeTo(timing.peakVelocity, 0.01));
    expect(g.segmentAtPointer(m.rotationAt(m.settlesAt!)), 4);
  });

  test('never turns backwards and slows without a jolt', () {
    final m = WheelMotion.start(rotation: 0, at: Duration.zero, timing: timing);
    m.land(at: ms(1000), geometry: g, slot: 9, within: 0.4);
    var prev = m.rotationAt(Duration.zero);
    for (var t = 0; t <= m.settlesAt!.inMilliseconds + 100; t += 8) {
      final r = m.rotationAt(ms(t));
      expect(r, greaterThanOrEqualTo(prev - 1e-12), reason: 'at $t ms');
      prev = r;
      if (t > 10 && t < m.settlesAt!.inMilliseconds - 10) {
        expect(m.velocityAt(ms(t)), closeTo(numericVelocity(m, ms(t)), 2e-3), reason: 'velocity continuous at $t ms');
      }
    }
  });

  test('a second answer for the same spin changes nothing', () {
    final m = WheelMotion.start(rotation: 0, at: Duration.zero, timing: timing);
    m.land(at: ms(500), geometry: g, slot: 2, within: 0.5);
    final target = m.target;
    final end = m.settlesAt;
    m.land(at: ms(900), geometry: g, slot: 7, within: 0.5);
    expect(m.target, target);
    expect(m.settlesAt, end);
  });

  test('stopping without an answer slows to rest quickly and claims no slot', () {
    final m = WheelMotion.start(rotation: 0, at: Duration.zero, timing: timing);
    m.stop(at: ms(2000));
    expect(m.target, isNull);
    expect(m.settlesAt! - ms(2000), lessThanOrEqualTo(const Duration(milliseconds: 1200)));
    expect(m.velocityAt(m.settlesAt!), closeTo(0, 1e-9));
    expect(m.hasOutcome, isFalse);
  });

  test('stopping after landing was planned keeps the planned stop', () {
    final m = WheelMotion.start(rotation: 0, at: Duration.zero, timing: timing);
    m.land(at: ms(500), geometry: g, slot: 3, within: 0.5);
    final end = m.settlesAt;
    m.stop(at: ms(600));
    expect(m.settlesAt, end);
    expect(m.hasOutcome, isTrue);
  });

  test('the plan uses no randomness: the same inputs give the same motion', () {
    WheelMotion plan() =>
        WheelMotion.start(rotation: 0.7, at: ms(10), timing: timing)
          ..land(at: ms(820), geometry: g, slot: 6, within: 0.61);
    final a = plan(), b = plan();
    for (var t = 0; t < 7000; t += 37) {
      expect(a.rotationAt(ms(t)), b.rotationAt(ms(t)));
    }
  });

  test('the peak speed stays readable (no more than about two turns a second)', () {
    expect(timing.peakVelocity, lessThanOrEqualTo(2.2 * 2 * pi));
  });
}
