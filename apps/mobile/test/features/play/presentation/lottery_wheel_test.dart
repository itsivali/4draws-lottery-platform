import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/theme/theme.dart';
import 'package:lottery_mobile/core/platform/haptics.dart';
import 'package:lottery_mobile/features/play/application/spin_haptics.dart';
import 'package:lottery_mobile/features/play/domain/wheel_geometry.dart';
import 'package:lottery_mobile/features/play/presentation/widgets/lottery_wheel.dart';
import 'package:lottery_mobile/features/play/presentation/widgets/wheel_fx_painters.dart';

import '../../../support/fonts.dart';

class Recorder implements Haptics {
  final events = <HapticEvent>[];
  @override
  void play(HapticEvent e) => events.add(e);
}

final server = WheelGeometry(slots: 12, luckySlots: 1);

Widget host({
  required WheelSpin spin,
  required SpinHaptics haptics,
  required VoidCallback onSettled,
  WheelGeometry? resting,
  double size = 360,
  bool reduceMotion = false,
}) => MaterialApp(
  debugShowCheckedModeBanner: false,
  theme: buildTheme(),
  home: MediaQuery(
    data: MediaQueryData(size: const Size(800, 800), disableAnimations: reduceMotion),
    child: Scaffold(
      body: Center(
        child: SizedBox.square(
          dimension: size,
          child: LotteryWheel(spin: spin, restingGeometry: resting, haptics: haptics, onSettled: onSettled),
        ),
      ),
    ),
  ),
);

void main() {
  setUpAll(loadAppFonts);

  LotteryWheelState wheel(WidgetTester t) => t.state<LotteryWheelState>(find.byType(LotteryWheel));

  testWidgets('spins while waiting, then stops on the server slot and reports it once', (t) async {
    final rec = Recorder();
    final haptics = SpinHaptics(rec);
    var settled = 0;
    await t.pumpWidget(host(spin: const WheelSpin.idle(), haptics: haptics, onSettled: () => settled++));
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k1'),
        haptics: haptics,
        onSettled: () => settled++,
      ),
    );
    await t.pump(const Duration(milliseconds: 900));
    expect(wheel(t).rotation, greaterThan(1), reason: 'turning while the answer is awaited');
    await t.pumpWidget(
      host(
        spin: WheelSpin(spinId: 'k1', slot: 7, geometry: server, within: 0.4),
        haptics: haptics,
        onSettled: () => settled++,
      ),
    );
    for (var i = 0; i < 400 && settled == 0; i++) {
      await t.pump(const Duration(milliseconds: 16));
    }
    expect(settled, 1);
    expect(server.segmentAtPointer(wheel(t).rotation), 7);
    await t.pump(const Duration(seconds: 2));
    expect(settled, 1);
    expect(rec.events.first, HapticEvent.start);
    expect(rec.events.last, HapticEvent.stop);
    expect(rec.events.where((e) => e == HapticEvent.stop), hasLength(1));
  });

  testWidgets('resizing during the spin does not change where it stops', (t) async {
    final haptics = SpinHaptics(Recorder());
    var settled = 0;
    void done() => settled++;
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k2'),
        haptics: haptics,
        onSettled: done,
      ),
    );
    await t.pump(const Duration(milliseconds: 500));
    await t.pumpWidget(
      host(
        spin: WheelSpin(spinId: 'k2', slot: 0, geometry: server),
        haptics: haptics,
        onSettled: done,
      ),
    );
    await t.pump(const Duration(milliseconds: 1500));
    await t.pumpWidget(
      host(
        spin: WheelSpin(spinId: 'k2', slot: 0, geometry: server),
        haptics: haptics,
        onSettled: done,
        size: 220,
      ),
    );
    for (var i = 0; i < 400 && settled == 0; i++) {
      await t.pump(const Duration(milliseconds: 16));
    }
    expect(server.segmentAtPointer(wheel(t).rotation), 0);
  });

  testWidgets('with reduced motion it does not spin and settles straight on the slot', (t) async {
    final rec = Recorder();
    final haptics = SpinHaptics(rec);
    var settled = 0;
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k3'),
        haptics: haptics,
        onSettled: () => settled++,
        reduceMotion: true,
      ),
    );
    await t.pump(const Duration(seconds: 1));
    expect(wheel(t).rotation, 0, reason: 'no motion while waiting');
    expect(find.byType(CircularProgressIndicator), findsOneWidget, reason: 'a still progress mark instead');
    await t.pumpWidget(
      host(
        spin: WheelSpin(spinId: 'k3', slot: 5, geometry: server),
        haptics: haptics,
        onSettled: () => settled++,
        reduceMotion: true,
      ),
    );
    await t.pump();
    await t.pump();
    expect(settled, 1);
    expect(server.segmentAtPointer(wheel(t).rotation), 5);
    expect(rec.events, [HapticEvent.start, HapticEvent.stop], reason: 'no ticks without motion');
  });

  testWidgets('a spin with no answer comes to rest without an outcome or stop feedback', (t) async {
    final rec = Recorder();
    final haptics = SpinHaptics(rec);
    var settled = 0;
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k4'),
        haptics: haptics,
        onSettled: () => settled++,
      ),
    );
    await t.pump(const Duration(seconds: 2));
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k4', abort: true),
        haptics: haptics,
        onSettled: () => settled++,
      ),
    );
    await t.pump(const Duration(seconds: 3));
    expect(settled, 0);
    expect(rec.events, isNot(contains(HapticEvent.stop)));
    final r = wheel(t).rotation;
    await t.pump(const Duration(seconds: 1));
    expect(wheel(t).rotation, r, reason: 'at rest');
  });

  testWidgets('a play recovered after its answer was lost gets its own spin onto the confirmed slot', (t) async {
    final haptics = SpinHaptics(Recorder());
    var settled = 0;
    void done() => settled++;
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k6'),
        haptics: haptics,
        onSettled: done,
      ),
    );
    await t.pump(const Duration(seconds: 2));
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k6', abort: true),
        haptics: haptics,
        onSettled: done,
      ),
    );
    await t.pump(const Duration(seconds: 3));
    expect(settled, 0);
    await t.pumpWidget(
      host(
        spin: WheelSpin(spinId: 'k6', slot: 4, geometry: server),
        haptics: haptics,
        onSettled: done,
      ),
    );
    for (var i = 0; i < 500 && settled == 0; i++) {
      await t.pump(const Duration(milliseconds: 16));
    }
    expect(settled, 1);
    expect(server.segmentAtPointer(wheel(t).rotation), 4);
  });

  testWidgets('with haptics turned off the spin is silent but otherwise the same', (t) async {
    final rec = Recorder();
    final haptics = SpinHaptics(GatedHaptics(rec, enabled: () => false));
    var settled = 0;
    await t.pumpWidget(
      host(
        spin: const WheelSpin(spinId: 'k5'),
        haptics: haptics,
        onSettled: () => settled++,
      ),
    );
    await t.pumpWidget(
      host(
        spin: WheelSpin(spinId: 'k5', slot: 2, geometry: server),
        haptics: haptics,
        onSettled: () => settled++,
      ),
    );
    for (var i = 0; i < 400 && settled == 0; i++) {
      await t.pump(const Duration(milliseconds: 16));
    }
    expect(settled, 1);
    expect(rec.events, isEmpty);
  });

  testWidgets('before the server has described the wheel it is plain decoration', (t) async {
    await t.pumpWidget(host(spin: const WheelSpin.idle(), haptics: SpinHaptics(Recorder()), onSettled: () {}));
    expect(wheel(t).geometry.luckySlots, 0);
    await t.pumpWidget(
      host(spin: const WheelSpin.idle(), haptics: SpinHaptics(Recorder()), onSettled: () {}, resting: server),
    );
    await t.pump();
    expect(wheel(t).geometry, server);
  });

  testWidgets('the wheel itself is hidden from screen readers (the result panel speaks)', (t) async {
    final h = t.ensureSemantics();
    await t.pumpWidget(
      host(spin: const WheelSpin.idle(), haptics: SpinHaptics(Recorder()), onSettled: () {}, resting: server),
    );
    expect(find.bySemanticsLabel(RegExp('.')), findsNothing);
    h.dispose();
  });

  group('animation (owner, 2026-10-30)', () {
    testWidgets('the lights run hot while spinning, cool at rest, and the landed slot glows once', (t) async {
      var settled = 0;
      final haptics = SpinHaptics(Recorder());
      await t.pumpWidget(
        host(spin: const WheelSpin.idle(), haptics: haptics, onSettled: () => settled++, resting: server),
      );
      await t.pumpWidget(
        host(
          spin: const WheelSpin(spinId: 's'),
          haptics: haptics,
          onSettled: () => settled++,
        ),
      );
      await t.pump(const Duration(milliseconds: 600));
      expect(wheel(t).fx.energy, greaterThan(0.9), reason: 'full speed: the lights race');
      final phase = wheel(t).fx.phase;
      await t.pump(const Duration(milliseconds: 200));
      expect(wheel(t).fx.phase, isNot(phase), reason: 'the lights move with the spin even without the idle loop');
      await t.pumpWidget(
        host(
          spin: WheelSpin(spinId: 's', slot: 5, geometry: server),
          haptics: haptics,
          onSettled: () => settled++,
        ),
      );
      await t.pump(const Duration(seconds: 4));
      await t.pump(const Duration(milliseconds: 400));
      expect(settled, 1);
      expect(wheel(t).fx.energy, 0);
      expect(wheel(t).fx.landedSlot, 5);
      expect(wheel(t).fx.celebrate, inExclusiveRange(0, 1), reason: 'the landing glow is under way');
      await t.pumpAndSettle();
      expect(wheel(t).fx.celebrate, 1, reason: 'and it ends, so the screen settles');
    });

    testWidgets('the pointer flicks as a stud passes, only while the wheel turns', (t) async {
      final sweep = server.sweep;
      expect(WheelPointerPainter.kick(rotation: 0, energy: 1, sweep: sweep), lessThan(-0.3), reason: 'on a stud');
      expect(
        WheelPointerPainter.kick(rotation: -sweep / 2, energy: 1, sweep: sweep),
        0,
        reason: 'mid-segment',
      );
      expect(WheelPointerPainter.kick(rotation: 0, energy: 0, sweep: sweep), 0, reason: 'at rest');
    });

    testWidgets('at rest the lights drift on their own, unless the platform asks for reduced motion', (t) async {
      LotteryWheel.ambientAnimation = true;
      addTearDown(() => LotteryWheel.ambientAnimation = false);
      await t.pumpWidget(
        host(spin: const WheelSpin.idle(), haptics: SpinHaptics(Recorder()), onSettled: () {}, resting: server),
      );
      final before = wheel(t).fx.phase;
      await t.pump(const Duration(milliseconds: 500));
      expect(wheel(t).fx.phase, isNot(before));
      await t.pumpWidget(
        host(
          spin: const WheelSpin.idle(),
          haptics: SpinHaptics(Recorder()),
          onSettled: () {},
          resting: server,
          reduceMotion: true,
        ),
      );
      final still = wheel(t).fx.phase;
      await t.pump(const Duration(milliseconds: 500));
      expect(wheel(t).fx.phase, still);
    });
  });

  group('golden', () {
    testWidgets('at rest', (t) async {
      await t.pumpWidget(
        host(spin: const WheelSpin.idle(), haptics: SpinHaptics(Recorder()), onSettled: () {}, resting: server),
      );
      await t.pump();
      await expectLater(find.byType(LotteryWheel), matchesGoldenFile('goldens/wheel_rest_light.png'));
    });
    testWidgets('settled on the lucky slot', (t) async {
      await t.pumpWidget(
        host(
          spin: const WheelSpin.idle(),
          haptics: SpinHaptics(Recorder()),
          onSettled: () {},
          resting: server,
          reduceMotion: true,
        ),
      );
      await t.pumpWidget(
        host(
          spin: WheelSpin(spinId: 'g', slot: 0, geometry: server, within: 0.5),
          haptics: SpinHaptics(Recorder()),
          onSettled: () {},
          reduceMotion: true,
        ),
      );
      await t.pump();
      await t.pump(const Duration(milliseconds: 300));
      await expectLater(find.byType(LotteryWheel), matchesGoldenFile('goldens/wheel_lucky.png'));
    });
  });
}
