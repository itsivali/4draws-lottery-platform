import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';
import 'depth.dart';
import 'reveal.dart';

/// One of the app's two hero surfaces (MASTER.md §5): the sunset gradient, lit from the top left, with a deep shadow. Text and
/// icons inside default to `onHero`. Used for the balance and for the wheel's stage, nowhere else.
class HeroCard extends StatelessWidget {
  const HeroCard({
    super.key,
    required this.child,
    this.padding = const EdgeInsets.all(Space.xl),
    this.radius = Radii.xl,
    this.stage = false,
  });

  final Widget child;
  final EdgeInsets padding;
  final double radius;

  /// The wheel's stage takes the dusk gradient (`Gradients.stage`) instead of the full sunset.
  final bool stage;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    return Reveal(
      child: Tilt(
        maxAngle: 0.05,
        radius: radius,
        child: DecoratedBox(
          decoration: BoxDecoration(
            gradient: stage ? Gradients.stage(c) : Gradients.hero(c),
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
        ),
      ),
    );
  }
}
