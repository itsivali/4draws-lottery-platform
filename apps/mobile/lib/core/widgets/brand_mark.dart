import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// The brand: the 4draws mark (an indigo tile with a white 4 in a ring of four segments) and the wordmark. Layouts
/// only give it a height.
class BrandMark extends StatelessWidget {
  const BrandMark({super.key, this.height = 28, this.showWordmark = true, this.wordmarkColor});

  static const name = '4draws';

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
          CustomPaint(size: Size.square(height), painter: const _MarkPainter()),
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

/// The 4draws mark (design-system/lottery-platform/logo/4draws-mark.svg): an indigo tile carrying a white 4 inside a
/// ring of four segments (the four draws), the top-right one gold. Drawn in a 64-unit box so it matches the icons
/// exactly; its colours are the logo's own, whatever the theme.
class _MarkPainter extends CustomPainter {
  const _MarkPainter();

  static const _tile = [Color(0xFF5B4FE9), Color(0xFF4338CA), Color(0xFF2A2385)];
  static const _gold = Color(0xFFFFC53D); // decorative, never text

  @override
  void paint(Canvas canvas, Size size) {
    final u = size.width / 64;
    final box = Offset.zero & size;
    canvas.drawRRect(
      RRect.fromRectAndRadius(box, Radius.circular(16 * u)),
      Paint()
        ..shader = const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: _tile,
          stops: [0, 0.55, 1],
        ).createShader(box),
    );
    final ring = Rect.fromCircle(center: box.center, radius: 22.5 * u);
    final stroke = Paint()
      ..style = PaintingStyle.stroke
      ..strokeCap = StrokeCap.round
      ..strokeWidth = 4.5 * u;
    const gap = 24 * math.pi / 180;
    for (var k = 0; k < 4; k++) {
      stroke.color = k == 0 ? _gold : Colors.white.withValues(alpha: 0.92);
      canvas.drawArc(ring, -math.pi / 2 + gap / 2 + k * math.pi / 2, math.pi / 2 - gap, false, stroke);
    }
    final four = Path()
      ..moveTo(36.5 * u, 44.5 * u)
      ..lineTo(36.5 * u, 19.5 * u)
      ..lineTo(22 * u, 38 * u)
      ..lineTo(43 * u, 38 * u);
    canvas.drawPath(
      four,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeCap = StrokeCap.round
        ..strokeJoin = StrokeJoin.round
        ..strokeWidth = 5.5 * u
        ..color = Colors.white,
    );
  }

  @override
  bool shouldRepaint(_MarkPainter old) => false;
}
