import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// Lifts a surface into place the first time it is built: a short fade with a rise of a few pixels, nothing more
/// (MASTER.md §7: motion explains, it never sells). Off when the platform asks for reduced motion.
class Reveal extends StatelessWidget {
  const Reveal({super.key, required this.child, this.delay = Duration.zero});

  final Widget child;

  /// Staggers surfaces that appear together (the second card a beat after the first).
  final Duration delay;

  @override
  Widget build(BuildContext context) {
    if (context.reduceMotion) return child;
    return TweenAnimationBuilder<double>(
      tween: Tween(begin: 0, end: 1),
      duration: Motion.standard * 2 + delay,
      curve: Interval(
        delay.inMilliseconds / (Motion.standard.inMilliseconds * 2 + delay.inMilliseconds),
        1,
        curve: Curves.easeOutCubic,
      ),
      child: child,
      builder: (context, t, child) => Opacity(
        opacity: t,
        child: Transform.translate(offset: Offset(0, 12 * (1 - t)), child: child),
      ),
    );
  }
}

/// Counts a sum up to its value when it first appears or changes, so a balance reads as money arriving, not a
/// number swapping. Off when the platform asks for reduced motion.
class CountUp extends StatelessWidget {
  const CountUp({super.key, required this.value, required this.format, this.style});

  final int value;
  final String Function(int) format;
  final TextStyle? style;

  @override
  Widget build(BuildContext context) {
    if (context.reduceMotion) return Text(format(value), style: style);
    return TweenAnimationBuilder<double>(
      tween: Tween(begin: 0, end: value.toDouble()),
      duration: Motion.standard * 3,
      curve: Curves.easeOutCubic,
      builder: (context, v, _) => Text(format(v.round()), style: style),
    );
  }
}
