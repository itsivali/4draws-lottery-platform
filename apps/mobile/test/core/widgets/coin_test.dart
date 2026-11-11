import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/theme/theme.dart';
import 'package:lottery_mobile/core/widgets/coin.dart';
import 'package:lottery_mobile/core/widgets/motion.dart';

void main() {
  Widget app(Widget child, {bool reduceMotion = false}) => MaterialApp(
    theme: buildTheme(),
    home: MediaQuery(
      data: MediaQueryData(disableAnimations: reduceMotion),
      child: Scaffold(body: Center(child: child)),
    ),
  );

  testWidgets('the coin is decoration: it adds nothing for screen readers', (t) async {
    final semantics = t.ensureSemantics();
    await t.pumpWidget(app(const Row(mainAxisSize: MainAxisSize.min, children: [Text('KES 1,250'), Coin3D()])));
    expect(find.bySemanticsLabel('KES 1,250'), findsOneWidget);
    expect(t.getSemantics(find.byType(Coin3D)).label, isEmpty);
    semantics.dispose();
  });

  testWidgets('the coin turns when motion is allowed', (t) async {
    Ambient.enabled = true;
    addTearDown(() => Ambient.enabled = false);
    await t.pumpWidget(app(const Coin3D()));
    expect(t.hasRunningAnimations, isTrue);
    await t.pumpWidget(const SizedBox());
  });

  testWidgets('under reduced motion it holds still, showing its edge', (t) async {
    Ambient.enabled = true;
    addTearDown(() => Ambient.enabled = false);
    await t.pumpWidget(app(const Coin3D(), reduceMotion: true));
    expect(t.hasRunningAnimations, isFalse);
    // the two faces and the milled edge's slices are all drawn
    expect(find.descendant(of: find.byType(Coin3D), matching: find.byType(CustomPaint)), findsAtLeastNWidgets(11));
  });
}
