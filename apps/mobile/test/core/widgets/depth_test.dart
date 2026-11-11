import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/theme/theme.dart';
import 'package:lottery_mobile/core/widgets/depth.dart';

void main() {
  Widget app(Widget child, {bool reduceMotion = false}) => MaterialApp(
    theme: buildTheme(),
    home: MediaQuery(
      data: MediaQueryData(disableAnimations: reduceMotion),
      child: Scaffold(body: Center(child: child)),
    ),
  );

  Matrix4? leanOf(WidgetTester t) {
    final transforms = find.descendant(of: find.byType(Tilt), matching: find.byType(Transform));
    return transforms.evaluate().isEmpty ? null : t.widget<Transform>(transforms.first).transform;
  }

  final card = Tilt(
    child: SizedBox(
      width: 200,
      height: 120,
      child: ElevatedButton(onPressed: () {}, child: const Text('Card')),
    ),
  );

  testWidgets('a card leans toward the finger while pressed and springs back flat on release', (t) async {
    await t.pumpWidget(app(card));
    expect(leanOf(t)!.isIdentity(), isTrue);

    final g = await t.startGesture(t.getTopLeft(find.byType(Tilt)) + const Offset(10, 10));
    await t.pumpAndSettle();
    expect(leanOf(t)!.isIdentity(), isFalse, reason: 'it leans toward the corner pressed');

    await g.up();
    await t.pumpAndSettle();
    expect(leanOf(t)!.storage.sublist(0, 12), Matrix4.identity().storage.sublist(0, 12));
  });

  testWidgets('the child keeps its own taps', (t) async {
    var taps = 0;
    await t.pumpWidget(
      app(
        Tilt(
          child: ElevatedButton(onPressed: () => taps++, child: const Text('Go')),
        ),
      ),
    );
    await t.tap(find.text('Go'));
    expect(taps, 1);
  });

  testWidgets('under reduced motion nothing leans', (t) async {
    await t.pumpWidget(app(card, reduceMotion: true));
    await t.startGesture(t.getTopLeft(find.byType(Tilt)) + const Offset(10, 10));
    await t.pumpAndSettle();
    expect(leanOf(t), isNull);
  });

  testWidgets('an orb is decoration: screen readers hear the words beside it, not the icon', (t) async {
    final semantics = t.ensureSemantics();
    await t.pumpWidget(
      app(
        const Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Orb(icon: Icons.add_rounded),
            Text('Top up'),
          ],
        ),
      ),
    );
    expect(find.bySemanticsLabel('Top up'), findsOneWidget);
    expect(find.byIcon(Icons.add_rounded), findsOneWidget);
    semantics.dispose();
  });

  testWidgets('pages cross-fade without moving under reduced motion', (t) async {
    final key = GlobalKey<NavigatorState>();
    await t.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        navigatorKey: key,
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(context).copyWith(disableAnimations: true),
          child: Grounded(child: child!),
        ),
        home: const Scaffold(body: Text('First')),
      ),
    );
    key.currentState!.push(MaterialPageRoute<void>(builder: (_) => const Scaffold(body: Text('Second'))));
    await t.pump();
    await t.pump(const Duration(milliseconds: 100));
    final moving = find.ancestor(of: find.text('Second'), matching: find.byType(Transform));
    expect(moving.evaluate().map((e) => (e.widget as Transform).transform).where((m) => !m.isIdentity()), isEmpty);
    await t.pumpAndSettle();
    expect(find.text('Second'), findsOneWidget);
  });

  testWidgets('every page stands on the one fluid ground, which stays still in tests', (t) async {
    await t.pumpWidget(
      MaterialApp(
        theme: buildTheme(),
        builder: (_, child) => Grounded(child: child!),
        home: const Text('Page'),
      ),
    );
    expect(find.byType(FluidBackdrop), findsOneWidget);
    expect(t.hasRunningAnimations, isFalse);
  });
}
