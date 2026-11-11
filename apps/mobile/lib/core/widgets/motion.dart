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
                  curve: Motion.settle,
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
    return Opacity(
      opacity: v.clamp(0.0, 1.0),
      child: Transform.translate(offset: Offset(0, Motion.riseDistance * (1 - v)), child: child),
    );
  }
}

/// Ambient motion that never ends (the turning coin). Tests turn it off so screens settle.
abstract final class Ambient {
  static bool enabled = true;
}

/// Material's fade-through between top-level destinations: when the chosen tab changes, its page fades in while it
/// settles from slightly small. Keeps the same child (the tabs keep their state); nothing happens under reduced motion.
class TabArrival extends StatefulWidget {
  const TabArrival({super.key, required this.index, required this.child});

  final int index;
  final Widget child;

  @override
  State<TabArrival> createState() => _TabArrivalState();
}

class _TabArrivalState extends State<TabArrival> with SingleTickerProviderStateMixin {
  late final _c = AnimationController(vsync: this, duration: Motion.emphasis, value: 1);
  late final _fade = CurvedAnimation(
    parent: _c,
    curve: const Interval(0, 0.7, curve: Motion.enter),
  );
  late final _scale = Tween(begin: 0.97, end: 1.0).animate(CurvedAnimation(parent: _c, curve: Motion.settle));

  @override
  void didUpdateWidget(TabArrival old) {
    super.didUpdateWidget(old);
    if (old.index != widget.index && !context.reduceMotion) _c.forward(from: 0);
  }

  @override
  void dispose() {
    _fade.dispose();
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => FadeTransition(
    opacity: _fade,
    child: ScaleTransition(scale: _scale, child: widget.child),
  );
}

/// A card that answers the pointer and the finger: it rises a little and its shadow deepens under a hovering pointer,
/// and it settles back down when pressed. Wraps a surface that draws no shadow of its own; only listens, so the child
/// keeps its taps. Under reduced motion the shadow still changes, without movement.
class Lift extends StatefulWidget {
  const Lift({super.key, required this.child, this.radius = Radii.card, this.enabled = true});

  final Widget child;
  final double radius;

  /// Off for cards that are not tappable: they keep the resting shadow.
  final bool enabled;

  @override
  State<Lift> createState() => _LiftState();
}

class _LiftState extends State<Lift> {
  bool _hover = false;
  bool _down = false;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final raised = widget.enabled && _hover && !_down;
    final still = context.reduceMotion;
    final card = AnimatedContainer(
      duration: still ? Duration.zero : Motion.standard,
      curve: Motion.enter,
      transform: Matrix4.translationValues(0, raised && !still ? -3 : 0, 0),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(widget.radius),
        boxShadow: raised ? Elevation.cardRaised(c) : Elevation.card(c),
      ),
      child: widget.child,
    );
    if (!widget.enabled) return card;
    return MouseRegion(
      onEnter: (_) => setState(() => _hover = true),
      onExit: (_) => setState(() => _hover = false),
      child: Listener(
        behavior: HitTestBehavior.deferToChild,
        onPointerDown: (_) => setState(() => _down = true),
        onPointerUp: (_) => setState(() => _down = false),
        onPointerCancel: (_) => setState(() => _down = false),
        child: AnimatedScale(
          scale: _down && !still ? Motion.pressScale : 1,
          duration: Motion.quick,
          curve: Motion.enter,
          child: card,
        ),
      ),
    );
  }
}
