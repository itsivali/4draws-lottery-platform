import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// A placeholder in the final layout's shape; it pulses slowly unless motion is reduced. Never fake numbers.
class Skeleton extends StatefulWidget {
  const Skeleton({super.key, this.width, this.height = 16, this.radius = Radii.sm});

  final double? width;
  final double height;
  final double radius;

  @override
  State<Skeleton> createState() => _SkeletonState();
}

class _SkeletonState extends State<Skeleton> with SingleTickerProviderStateMixin {
  late final _c = AnimationController(vsync: this, duration: Motion.skeletonPulse);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (context.reduceMotion) {
      _c.stop();
    } else if (!_c.isAnimating) {
      _c.repeat(reverse: true);
    }
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final box = Container(
      width: widget.width,
      height: widget.height,
      decoration: BoxDecoration(color: context.colors.surfaceMuted, borderRadius: BorderRadius.circular(widget.radius)),
    );
    return ExcludeSemantics(
      child: context.reduceMotion ? box : FadeTransition(opacity: Tween(begin: 1.0, end: 0.55).animate(_c), child: box),
    );
  }
}

/// A few skeleton lines, labelled once for screen readers.
class SkeletonLines extends StatelessWidget {
  const SkeletonLines({super.key, this.lines = 3, this.label = 'Loading'});

  final int lines;
  final String label;

  @override
  Widget build(BuildContext context) => Semantics(
    label: label,
    liveRegion: true,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (var i = 0; i < lines; i++) ...[
          if (i > 0) const SizedBox(height: Space.sm),
          FractionallySizedBox(widthFactor: i == lines - 1 ? 0.6 : 1, child: const Skeleton()),
        ],
      ],
    ),
  );
}
