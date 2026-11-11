import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/theme/theme.dart';
import 'package:lottery_mobile/app/theme/tokens.dart';
import 'package:lottery_mobile/core/widgets/app_card.dart';
import 'package:lottery_mobile/core/widgets/auth_page.dart';
import 'package:lottery_mobile/core/widgets/motion.dart';

void main() {
  Widget app(Widget child) => MaterialApp(
    theme: buildTheme(),
    home: Scaffold(body: Center(child: child)),
  );

  List<BoxShadow>? shadowOf(WidgetTester t) {
    final box = t.widget<AnimatedContainer>(
      find.descendant(of: find.byType(Lift), matching: find.byType(AnimatedContainer)),
    );
    return (box.decoration! as BoxDecoration).boxShadow;
  }

  testWidgets('every raised card stands on a shadow; a tappable one comes forward under the pointer', (t) async {
    var taps = 0;
    await t.pumpWidget(app(AppCard(onTap: () => taps++, child: const Text('Open'))));
    const c = AppColors.light;
    expect(shadowOf(t), Elevation.card(c));

    final mouse = await t.createGesture(kind: PointerDeviceKind.mouse);
    await mouse.addPointer(location: Offset.zero);
    addTearDown(mouse.removePointer);
    await mouse.moveTo(t.getCenter(find.text('Open')));
    await t.pumpAndSettle();
    expect(shadowOf(t), Elevation.cardRaised(c));

    await t.tap(find.text('Open'));
    expect(taps, 1, reason: 'the card keeps its tap');
  });

  testWidgets('a card that is not tappable keeps its resting shadow', (t) async {
    await t.pumpWidget(app(const AppCard(child: Text('Facts'))));
    final mouse = await t.createGesture(kind: PointerDeviceKind.mouse);
    await mouse.addPointer(location: Offset.zero);
    addTearDown(mouse.removePointer);
    await mouse.moveTo(t.getCenter(find.text('Facts')));
    await t.pumpAndSettle();
    expect(shadowOf(t), Elevation.card(AppColors.light));
  });

  testWidgets('the signed-out page centres one card on the brand colour, at any width', (t) async {
    for (final size in const [Size(390, 844), Size(1440, 900)]) {
      t.view.physicalSize = size;
      t.view.devicePixelRatio = 1;
      addTearDown(t.view.reset);
      await t.pumpWidget(
        MaterialApp(
          theme: buildTheme(),
          home: const AuthPage(title: 'Good to see you', lead: 'Sign in.', child: SizedBox(height: 120)),
        ),
      );
      await t.pumpAndSettle();
      final card = t.getRect(find.ancestor(of: find.text('Good to see you'), matching: find.byType(Material)).first);
      expect(card.center.dx, moreOrLessEquals(size.width / 2, epsilon: 0.5), reason: '$size: centred');
      expect(card.width, lessThanOrEqualTo(440));
      expect(t.widget<Scaffold>(find.byType(Scaffold)).backgroundColor, AppColors.light.heroEnd);
    }
  });
}
