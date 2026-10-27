import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// One of the app's two hero surfaces (MASTER.md §5): deep emerald, lit from the top left, with a deep shadow. Text and
/// icons inside default to `onHero`. Used for the balance and for the wheel's stage, nowhere else.
class HeroCard extends StatelessWidget {
  const HeroCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(Space.xl),
    this.radius = Radii.xl,
    this.ornament = true,
  });

  final Widget child;
  final EdgeInsets padding;
  final double radius;

  /// The faint concentric rings in the corner (off for the wheel, which brings its own).
  final bool ornament;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: Gradients.hero(c),
        borderRadius: BorderRadius.circular(radius),
        boxShadow: Elevation.hero(c),
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(radius),
        child: Stack(
          children: [
            Positioned.fill(
              child: DecoratedBox(decoration: BoxDecoration(gradient: Gradients.heroGlow(c))),
            ),
            if (ornament)
              Positioned(
                right: -radius * 2,
                top: -radius * 2.5,
                child: ExcludeSemantics(
                  child: CustomPaint(size: const Size(220, 220), painter: _RingsPainter(c.onHero)),
                ),
              ),
            Padding(
              padding: padding,
              child: DefaultTextStyle.merge(
                style: t.bodyMedium?.copyWith(color: c.onHero),
                child: IconTheme.merge(
                  data: IconThemeData(color: c.onHero),
                  child: child,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _RingsPainter extends CustomPainter {
  _RingsPainter(this.color);

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final centre = Offset(size.width / 2, size.height / 2);
    final paint = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;
    for (final (r, a) in [(size.width / 2, 0.10), (size.width / 2.8, 0.08), (size.width / 4.6, 0.06)]) {
      canvas.drawCircle(centre, r, paint..color = color.withValues(alpha: a));
    }
  }

  @override
  bool shouldRepaint(_RingsPainter old) => old.color != color;
}
