import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// Shrinks its child slightly while pressed (MASTER.md §8). Nothing happens under reduced motion; the child keeps its
/// own tap handling, this only listens.
class PressScale extends StatefulWidget {
  const PressScale({super.key, required this.child, this.enabled = true});

  final Widget child;
  final bool enabled;

  @override
  State<PressScale> createState() => _PressScaleState();
}

class _PressScaleState extends State<PressScale> {
  bool _down = false;

  void _set(bool v) {
    if (_down != v) setState(() => _down = v);
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.enabled || context.reduceMotion) return widget.child;
    return Listener(
      behavior: HitTestBehavior.deferToChild,
      onPointerDown: (_) => _set(true),
      onPointerUp: (_) => _set(false),
      onPointerCancel: (_) => _set(false),
      child: AnimatedScale(
        scale: _down ? Motion.pressScale : 1,
        duration: Motion.press,
        curve: Motion.enter,
        child: widget.child,
      ),
    );
  }
}

/// Fades and lifts each child into place, one after another (MASTER.md §8). Lists arrive as a whole, not from nowhere.
/// Under reduced motion the children are simply there.
class Staggered extends StatefulWidget {
  const Staggered({
    super.key,
    required this.children,
    this.spacing = Space.lg,
    this.crossAxisAlignment = CrossAxisAlignment.stretch,
  });

  final List<Widget> children;
  final double spacing;
  final CrossAxisAlignment crossAxisAlignment;

  @override
  State<Staggered> createState() => _StaggeredState();
}

class _StaggeredState extends State<Staggered> with SingleTickerProviderStateMixin {
  late final AnimationController _c = AnimationController(
    vsync: this,
    duration: Motion.emphasis + Motion.stagger * widget.children.length,
  )..forward();

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final reduce = context.reduceMotion;
    final total = _c.duration!.inMilliseconds.toDouble();
    return Column(
      crossAxisAlignment: widget.crossAxisAlignment,
      children: [
        for (var i = 0; i < widget.children.length; i++) ...[
          if (i > 0) SizedBox(height: widget.spacing),
          if (reduce)
            widget.children[i]
          else
            _Enter(
              animation: CurvedAnimation(
                parent: _c,
                curve: Interval(
                  (Motion.stagger.inMilliseconds * i) / total,
                  ((Motion.stagger.inMilliseconds * i) + Motion.emphasis.inMilliseconds) / total,
                  curve: Curves.easeOutBack,
                ),
              ),
              child: widget.children[i],
            ),
        ],
      ],
    );
  }
}

class _Enter extends AnimatedWidget {
  const _Enter({required Animation<double> animation, required this.child}) : super(listenable: animation);

  final Widget child;

  @override
  Widget build(BuildContext context) {
    final v = (listenable as Animation<double>).value;
    // each card swings up out of a backward lean as it rises, so a list unfolds in depth (redesign, second pass)
    return Opacity(
      opacity: v.clamp(0.0, 1.0),
      child: Transform(
        alignment: Alignment.topCenter,
        transform: Matrix4.identity()
          ..setEntry(3, 2, 0.0011)
          ..translateByDouble(0, Motion.riseDistance * 2.4 * (1 - v), 0, 1)
          ..rotateX(0.42 * (1 - v))
          ..scaleByDouble(0.94 + 0.06 * v, 0.94 + 0.06 * v, 1, 1),
        child: child,
      ),
    );
  }
}
