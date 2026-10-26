import 'dart:math';

import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// A placeholder brand: an emerald disc with a gold pointer notch, and the wordmark. Replace this widget with the final
/// logo; layouts only give it a height.
class BrandMark extends StatelessWidget {
  const BrandMark({super.key, this.height = 28, this.showWordmark = true});

  static const name = 'Bahati';

  final double height;
  final bool showWordmark;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return Semantics(
      label: name,
      excludeSemantics: true,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          CustomPaint(size: Size.square(height), painter: _DiscPainter(c.primary, c.gold)),
          if (showWordmark) ...[
            SizedBox(width: height * 0.32),
            Text(
              name,
              style: Theme.of(context).textTheme.titleLarge?.copyWith(fontSize: height * 0.72, height: 1, color: c.ink),
            ),
          ],
        ],
      ),
    );
  }
}

class _DiscPainter extends CustomPainter {
  _DiscPainter(this.disc, this.notch);

  final Color disc;
  final Color notch;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.width / 2;
    final centre = Offset(r, r);
    canvas.drawCircle(centre, r, Paint()..color = disc);
    final ring = Paint()
      ..color = notch
      ..style = PaintingStyle.stroke
      ..strokeWidth = r * 0.14;
    canvas.drawArc(Rect.fromCircle(center: centre, radius: r * 0.62), -pi / 2 + 0.5, 2 * pi - 1.0, false, ring);
    final path = Path()
      ..moveTo(r, r * 0.12)
      ..lineTo(r - r * 0.26, -r * 0.08)
      ..lineTo(r + r * 0.26, -r * 0.08)
      ..close();
    canvas.drawPath(path.shift(Offset(0, r * 0.2)), Paint()..color = notch);
  }

  @override
  bool shouldRepaint(_DiscPainter old) => old.disc != disc || old.notch != notch;
}
