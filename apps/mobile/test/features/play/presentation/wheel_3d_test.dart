import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/theme/theme.dart';
import 'package:lottery_mobile/app/theme/tokens.dart';
import 'package:lottery_mobile/features/play/presentation/widgets/lottery_wheel.dart';
import 'package:lottery_mobile/features/play/presentation/widgets/wheel_fx_painters.dart';

void main() {
  Widget wheel(WheelFx fx, {bool still = false}) => MaterialApp(
    theme: buildTheme(),
    home: Center(
      child: SizedBox.square(
        dimension: 300,
        child: Builder(
          builder: (context) => Wheel3D(
            fx: fx,
            colors: context.colors,
            still: still,
            child: const ColoredBox(key: Key('face'), color: Colors.amber),
          ),
        ),
      ),
    ),
  );

  Matrix4 faceTransform(WidgetTester t) => t
      .widget<Transform>(find.ancestor(of: find.byKey(const Key('face')), matching: find.byType(Transform)).first)
      .transform;

  testWidgets('the wheel stands in 3D: a rim of slices behind a leaning face', (t) async {
    await t.pumpWidget(wheel(WheelFx()));
    expect(faceTransform(t).isIdentity(), isFalse, reason: 'it leans back');
    expect(find.descendant(of: find.byType(Wheel3D), matching: find.byType(Transform)), findsNWidgets(13));
  });

  testWidgets('it leans toward the finger, and back when let go', (t) async {
    final fx = WheelFx();
    await t.pumpWidget(wheel(fx));
    final rest = faceTransform(t).clone();
    final g = await t.startGesture(t.getCenter(find.byType(Wheel3D)) + const Offset(120, 0));
    await t.pumpAndSettle();
    expect(faceTransform(t), isNot(rest));
    await g.up();
    await t.pumpAndSettle();
    for (var i = 0; i < 16; i++) {
      expect(faceTransform(t).storage[i], moreOrLessEquals(rest.storage[i], epsilon: 1e-6));
    }
  });

  testWidgets('held still (reduced motion), the pose ignores the finger and the spin', (t) async {
    final fx = WheelFx();
    await t.pumpWidget(wheel(fx, still: true));
    final rest = faceTransform(t).clone();
    fx.update(energy: 1, rotation: 1.3);
    await t.startGesture(t.getCenter(find.byType(Wheel3D)) + const Offset(120, 0));
    await t.pumpAndSettle();
    expect(faceTransform(t), rest);
  });
}
