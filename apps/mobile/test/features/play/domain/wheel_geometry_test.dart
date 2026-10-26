import 'dart:math';

import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/features/play/domain/wheel_geometry.dart';

const tau = 2 * pi;

void main() {
  group('normalizeAngle', () {
    test('maps any angle into [0, 2π)', () {
      expect(normalizeAngle(0), 0);
      expect(normalizeAngle(tau), closeTo(0, 1e-12));
      expect(normalizeAngle(-0.25), closeTo(tau - 0.25, 1e-12));
      expect(normalizeAngle(5 * tau + 1), closeTo(1, 1e-9));
      expect(normalizeAngle(-7 * tau - 1), closeTo(tau - 1, 1e-9));
    });
  });

  group('WheelGeometry', () {
    test('refuses fewer than two slots and lucky slots outside the wheel', () {
      expect(() => WheelGeometry(slots: 1), throwsArgumentError);
      expect(() => WheelGeometry(slots: 12, luckySlots: 13), throwsArgumentError);
      expect(() => WheelGeometry(slots: 12, luckySlots: -1), throwsArgumentError);
    });

    test('segments split the circle evenly, clockwise from the pointer', () {
      final g = WheelGeometry(slots: 12);
      expect(g.sweep, closeTo(tau / 12, 1e-12));
      expect(g.segmentStart(0), 0);
      expect(g.segmentStart(3), closeTo(3 * tau / 12, 1e-12));
      expect(g.segmentCenter(0), closeTo(tau / 24, 1e-12));
    });

    test('slots below lucky_slots are lucky, as the API defines', () {
      final g = WheelGeometry(slots: 12, luckySlots: 2);
      expect([for (var i = 0; i < 12; i++) g.isLucky(i)].where((l) => l).length, 2);
      expect(g.isLucky(0), isTrue);
      expect(g.isLucky(1), isTrue);
      expect(g.isLucky(2), isFalse);
    });

    test('at rest the pointer sits on the start of segment 0', () {
      final g = WheelGeometry(slots: 12);
      expect(g.segmentAtPointer(0), 0);
      // turning the wheel clockwise by a little brings the end of the last segment under the pointer
      expect(g.segmentAtPointer(0.01), 11);
      expect(g.segmentAtPointer(-0.01), 0);
    });

    for (final slots in [2, 3, 7, 12, 37]) {
      test('the resting rotation for every slot of $slots puts that slot under the pointer', () {
        final g = WheelGeometry(slots: slots);
        for (var i = 0; i < slots; i++) {
          for (final within in [0.25, 0.5, 0.75]) {
            final r = g.restingRotationFor(i, within: within);
            expect(r, inInclusiveRange(0, tau));
            expect(g.segmentAtPointer(r), i, reason: 'slot $i within $within');
            // any number of whole turns lands on the same slot
            expect(g.segmentAtPointer(r + 9 * tau), i);
            expect(g.segmentAtPointer(r - 4 * tau), i);
          }
        }
      });
    }

    test('targets land on the slot after at least the minimum travel, from any start', () {
      final g = WheelGeometry(slots: 12);
      for (final from in [0.0, 0.5, tau - 1e-9, 3 * tau + 2, -1.0]) {
        for (var slot = 0; slot < 12; slot++) {
          final t = g.targetRotation(from: from, segment: slot, minTravel: 3 * tau);
          expect(t - from, greaterThanOrEqualTo(3 * tau));
          expect(t - from, lessThan(4 * tau));
          expect(g.segmentAtPointer(t), slot, reason: 'from $from to slot $slot');
        }
      }
    });

    test('a target stays on its slot near segment boundaries (no floating-point drift)', () {
      final g = WheelGeometry(slots: 12);
      for (var slot = 0; slot < 12; slot++) {
        final t = g.targetRotation(from: 1e6, segment: slot, minTravel: 0, within: 0.02);
        expect(g.segmentAtPointer(t), slot);
        final u = g.targetRotation(from: 1e6, segment: slot, minTravel: 0, within: 0.98);
        expect(g.segmentAtPointer(u), slot);
      }
    });

    test('counts the boundaries the pointer crosses between two rotations', () {
      final g = WheelGeometry(slots: 12);
      final s = g.sweep;
      expect(g.boundariesCrossed(0, 0), 0);
      expect(g.boundariesCrossed(0.1 * s, 0.9 * s), 0);
      expect(g.boundariesCrossed(0.9 * s, 1.1 * s), 1);
      expect(g.boundariesCrossed(0.5 * s, 12.5 * s), 12);
      expect(g.boundariesCrossed(-0.5 * s, 0.5 * s), 1, reason: 'wraps through zero');
    });

    test('geometry depends only on slot count, never on the size it is drawn at', () {
      // resizing redraws the same angles: the painter scales a unit wheel
      final a = WheelGeometry(slots: 12);
      final b = WheelGeometry(slots: 12);
      expect(a.restingRotationFor(5), b.restingRotationFor(5));
      expect(a, b);
    });
  });

  group('landingFraction', () {
    test('is deterministic per ticket and keeps clear of segment edges', () {
      final f = landingFraction('0192f000-0000-7000-8000-000000000001');
      expect(f, landingFraction('0192f000-0000-7000-8000-000000000001'));
      final seen = <double>{};
      for (var i = 0; i < 500; i++) {
        final v = landingFraction('ticket-$i');
        expect(v, inInclusiveRange(0.25, 0.75));
        seen.add(v);
      }
      expect(seen.length, greaterThan(100), reason: 'tickets should not all stop at the same spot');
    });
  });
}
