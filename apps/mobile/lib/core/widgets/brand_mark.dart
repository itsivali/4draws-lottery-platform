import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// The brand: the Tajirika mark (a sunset tile with a white T and the wheel's gold pointer) and the wordmark. Layouts
/// only give it a height.
class BrandMark extends StatelessWidget {
  const BrandMark({super.key, this.height = 28, this.showWordmark = true, this.wordmarkColor});

  static const name = 'Tajirika';

  final double height;
  final bool showWordmark;

  /// The wordmark's colour; ink by default, `onHero` on a hero surface.
  final Color? wordmarkColor;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return Semantics(
      label: name,
      excludeSemantics: true,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          CustomPaint(size: Size.square(height), painter: _MarkPainter(c)),
          if (showWordmark) ...[
            SizedBox(width: height * 0.32),
            Text(
              name,
              style: Theme.of(context).textTheme.titleLarge?.copyWith(
                fontSize: height * 0.74,
                fontWeight: FontWeight.w800,
                letterSpacing: -height * 0.02,
                height: 1,
                color: wordmarkColor ?? c.ink,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

/// The Tajirika mark (design-system/lottery-platform/logo/tajirika-mark.svg): a sunset tile carrying a white T, with
/// the wheel's pointer above it in decorative gold. Drawn in a 64-unit box so it matches the icons exactly.
class _MarkPainter extends CustomPainter {
  _MarkPainter(this.c);

  final AppColors c;

  @override
  void paint(Canvas canvas, Size size) {
    final u = size.width / 64;
    final tile = RRect.fromRectAndRadius(Offset.zero & size, Radius.circular(16 * u));
    canvas.drawRRect(
      tile,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [c.heroStart, c.primary, c.heroEnd],
          stops: const [0, 0.62, 1],
        ).createShader(Offset.zero & size),
    );
    final white = Paint()..color = Colors.white;
    canvas.drawRRect(
      RRect.fromRectAndRadius(Rect.fromLTWH(15 * u, 17 * u, 34 * u, 10 * u), Radius.circular(3 * u)),
      white,
    );
    canvas.drawRRect(
      RRect.fromRectAndRadius(Rect.fromLTWH(27 * u, 17 * u, 10 * u, 33 * u), Radius.circular(3 * u)),
      white,
    );
    final pointer = Path()
      ..moveTo(32 * u, 7.5 * u)
      ..lineTo(36.6 * u, 14 * u)
      ..lineTo(27.4 * u, 14 * u)
      ..close();
    canvas.drawPath(pointer, Paint()..color = const Color(0xFFF2C14E)); // decorative gold, never text
  }

  @override
  bool shouldRepaint(_MarkPainter old) => old.c != c;
}
