import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/platform/haptics.dart';
import 'package:lottery_mobile/features/play/application/spin_haptics.dart';
import 'package:lottery_mobile/features/play/domain/wheel_geometry.dart';
import 'package:lottery_mobile/features/play/domain/wheel_motion.dart';

class RecordingHaptics implements Haptics {
  final events = <(HapticEvent, Duration?)>[];
  Duration? now;

  @override
  void play(HapticEvent event) => events.add((event, now));

  List<HapticEvent> get kinds => [for (final e in events) e.$1];
}

Duration ms(num v) => Duration(microseconds: (v * 1000).round());

void main() {
  final geometry = WheelGeometry(slots: 12, luckySlots: 1);

  /// Drives a whole spin at 60 frames a second, like the wheel widget does, and returns what was felt.
  RecordingHaptics simulate({required int slot, Duration answerAt = const Duration(milliseconds: 300)}) {
    final h = RecordingHaptics();
    final spin = SpinHaptics(h);
    final motion = WheelMotion.start(rotation: 0, at: Duration.zero);
    spin.begin('ticket-1');
    var t = Duration.zero;
    while (true) {
      if (t >= answerAt && motion.target == null) {
        motion.land(at: t, geometry: geometry, slot: slot, within: 0.5);
      }
      h.now = t;
      spin.onFrame(
        at: t,
        geometry: geometry,
        rotation: motion.rotationAt(t),
        velocity: motion.velocityAt(t),
        phase: motion.phaseAt(t),
      );
      if (motion.settledAt(t)) {
        spin.onSettled(hasOutcome: motion.hasOutcome);
        break;
      }
      t += const Duration(microseconds: 16667);
    }
    return h;
  }

  test('a spin is acknowledged once, however many times the play is started', () {
    final h = RecordingHaptics();
    final spin = SpinHaptics(h);
    spin.begin('ticket-1');
    spin.begin('ticket-1');
    spin.begin('ticket-1');
    expect(h.kinds, [HapticEvent.start]);
    spin.begin('ticket-2');
    expect(h.kinds, [HapticEvent.start, HapticEvent.start]);
  });

  test('no ticks while the wheel is fast; ticks only as it slows, at pointer crossings', () {
    final h = simulate(slot: 7);
    final ticks = h.events.where((e) => e.$1 == HapticEvent.tick).toList();
    expect(ticks, isNotEmpty);
    // every tick comes after slowing began (the answer arrives at 300 ms; slowing starts at 1.8 s at the earliest)
    expect(ticks.every((e) => e.$2! > const Duration(milliseconds: 1800)), isTrue);
  });

  test('ticks are spaced and capped so the motor never buzzes', () {
    for (var slot = 0; slot < 12; slot++) {
      final h = simulate(slot: slot);
      final ticks = [
        for (final e in h.events)
          if (e.$1 == HapticEvent.tick) e.$2!,
      ];
      expect(ticks.length, lessThanOrEqualTo(SpinHaptics.maxTicks));
      for (var i = 1; i < ticks.length; i++) {
        expect(ticks[i] - ticks[i - 1], greaterThanOrEqualTo(SpinHaptics.minGap), reason: 'slot $slot tick $i');
      }
    }
  });

  test('the stop is felt exactly once, and last', () {
    final h = simulate(slot: 3);
    expect(h.kinds.where((k) => k == HapticEvent.stop).length, 1);
    expect(h.kinds.last, HapticEvent.stop);
    expect(h.kinds.first, HapticEvent.start);
  });

  test('a spin that ends without an answer gives no stop feedback', () {
    final h = RecordingHaptics();
    final spin = SpinHaptics(h);
    spin.begin('ticket-9');
    spin.onSettled(hasOutcome: false);
    spin.onSettled(hasOutcome: true); // a late call for the same spin is ignored
    expect(h.kinds, [HapticEvent.start]);
  });

  test('a tick needs a real crossing: frames inside one segment are silent', () {
    final h = RecordingHaptics();
    final spin = SpinHaptics(h);
    spin.begin('t');
    final s = geometry.sweep;
    for (var i = 0; i < 10; i++) {
      spin.onFrame(
        at: ms(2000 + i * 100),
        geometry: geometry,
        rotation: s * (0.1 + i * 0.05),
        velocity: 0.5,
        phase: MotionPhase.decelerating,
      );
    }
    expect(h.kinds, [HapticEvent.start]);
    spin.onFrame(at: ms(4000), geometry: geometry, rotation: s * 1.05, velocity: 0.5, phase: MotionPhase.decelerating);
    expect(h.kinds, [HapticEvent.start, HapticEvent.tick]);
  });

  test('several crossings in one frame give one tick at most', () {
    final h = RecordingHaptics();
    final spin = SpinHaptics(h);
    spin.begin('t');
    spin.onFrame(at: ms(2000), geometry: geometry, rotation: 0.01, velocity: 1, phase: MotionPhase.decelerating);
    spin.onFrame(
      at: ms(2100),
      geometry: geometry,
      rotation: geometry.sweep * 3.5,
      velocity: 1,
      phase: MotionPhase.decelerating,
    );
    expect(h.kinds.where((k) => k == HapticEvent.tick).length, 1);
  });

  group('GatedHaptics', () {
    test('passes feedback through only while enabled', () {
      final inner = RecordingHaptics();
      var on = true;
      final gated = GatedHaptics(inner, enabled: () => on);
      gated.play(HapticEvent.tick);
      on = false;
      gated.play(HapticEvent.stop);
      gated.play(HapticEvent.confirmed);
      expect(inner.kinds, [HapticEvent.tick]);
    });
  });

  group('hapticsForPlatform', () {
    test('the web never gets device haptics', () {
      expect(hapticsForPlatform(isWeb: true, enabled: () => true), isA<SilentHaptics>());
    });
    test('phones get platform haptics behind the setting', () {
      expect(hapticsForPlatform(isWeb: false, enabled: () => true), isA<GatedHaptics>());
    });
  });
}
