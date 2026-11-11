import 'dart:math' as math;

import 'package:flutter/gestures.dart' show PointerDeviceKind;
import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

// Depth (redesign 2026-11-11, second pass): surfaces live in 3D space. Cards and tiles lean toward the finger or the
// pointer and catch the light; icons are glossy orbs; pages arrive from depth and recede behind the next; a slow fluid
// of coral and violet moves behind every page. All of it is perspective transforms and gradients drawn in code, so it
// costs no assets and runs the same on phones and the web. Under reduced motion every surface is flat and still, and
// nothing here ever carries meaning on its own.

/// Ambient motion that never ends (the drifting backdrop, floating orbs). Tests turn it off so screens settle.
abstract final class Ambient {
  static bool enabled = true;
}

/// The perspective every 3D transform uses: about a phone's width away from the eye.
const double _perspective = 0.0011;

Matrix4 _tilt(double x, double y, double maxAngle) => Matrix4.identity()
  ..setEntry(3, 2, _perspective)
  ..rotateX(-y * maxAngle)
  ..rotateY(x * maxAngle);

/// Leans its child toward the pointer (hover on the web and desktop) or toward the finger while pressed, sinks it a
/// little under a press, and springs back on release. A soft highlight follows the lean, as if the surface caught the
/// light. Only listens: the child keeps its own taps. Flat under reduced motion.
class Tilt extends StatefulWidget {
  const Tilt({
    super.key,
    required this.child,
    this.maxAngle = 0.08,
    this.pressScale = 0.975,
    this.radius = Radii.card,
    this.sheen = true,
    this.enabled = true,
  });

  final Widget child;

  /// How far it leans at the edge, in radians.
  final double maxAngle;
  final double pressScale;

  /// The corners the highlight is clipped to.
  final double radius;
  final bool sheen;
  final bool enabled;

  @override
  State<Tilt> createState() => _TiltState();
}

class _TiltState extends State<Tilt> {
  Offset _aim = Offset.zero;
  bool _down = false;

  void _point(Offset local) {
    final size = context.size;
    if (size == null || size.isEmpty) return;
    final x = (local.dx / size.width * 2 - 1).clamp(-1.0, 1.0);
    final y = (local.dy / size.height * 2 - 1).clamp(-1.0, 1.0);
    setState(() => _aim = Offset(x, y));
  }

  void _rest() => setState(() {
    _aim = Offset.zero;
    _down = false;
  });

  @override
  Widget build(BuildContext context) {
    if (!widget.enabled || context.reduceMotion) return widget.child;
    return MouseRegion(
      onHover: (e) => _point(e.localPosition),
      onExit: (_) => _rest(),
      child: Listener(
        behavior: HitTestBehavior.deferToChild,
        onPointerDown: (e) {
          _down = true;
          _point(e.localPosition);
        },
        // a finger dragging a list scrolls it; only a mouse keeps steering the lean
        onPointerMove: (e) {
          if (e.kind == PointerDeviceKind.mouse) _point(e.localPosition);
        },
        onPointerUp: (_) => _rest(),
        onPointerCancel: (_) => _rest(),
        child: TweenAnimationBuilder<Offset>(
          tween: Tween(end: _aim),
          duration: Motion.emphasis,
          curve: _aim == Offset.zero ? Curves.easeOutBack : Curves.easeOutCubic,
          child: widget.child,
          builder: (context, v, child) {
            final lean = v.distance.clamp(0.0, 1.4);
            return AnimatedScale(
              scale: _down ? widget.pressScale : 1,
              duration: Motion.quick,
              curve: Motion.enter,
              child: Transform(
                alignment: Alignment.center,
                transform: _tilt(v.dx, v.dy, widget.maxAngle),
                child: !widget.sheen || lean < 0.01
                    ? child
                    : Stack(
                        children: [
                          child!,
                          Positioned.fill(
                            child: IgnorePointer(
                              child: ClipRRect(
                                borderRadius: BorderRadius.circular(widget.radius),
                                child: DecoratedBox(
                                  decoration: BoxDecoration(
                                    gradient: RadialGradient(
                                      center: Alignment(v.dx, v.dy),
                                      radius: 1.1,
                                      colors: [
                                        Colors.white.withValues(alpha: 0.20 * lean),
                                        Colors.white.withValues(alpha: 0),
                                      ],
                                    ),
                                  ),
                                ),
                              ),
                            ),
                          ),
                        ],
                      ),
              ),
            );
          },
        ),
      ),
    );
  }
}

/// The orbs' finishes: a colour lit from the top left, glossy, with a coloured shadow under it.
enum OrbTone { brand, soft, gold, success, warning, error, pending, ink }

/// An icon as a glossy 3D sphere: a radial body lit from the top left, a specular highlight, a coloured shadow
/// beneath and a white glyph with its own small shadow. `soft` is a pale pearl with a coloured glyph, for quiet rows.
/// Decorative: say what it means in text beside it.
class Orb extends StatelessWidget {
  const Orb({super.key, required this.icon, this.size = 44, this.tone = OrbTone.brand, this.color});

  final IconData icon;
  final double size;
  final OrbTone tone;

  /// Overrides the tone's colour (a list row's own colour).
  final Color? color;

  (Color, Color) _body(AppColors c) => switch (tone) {
    OrbTone.brand => (c.primary, c.accent),
    OrbTone.soft => (c.surface, c.surfaceMuted),
    OrbTone.gold => (c.gold, c.goldText),
    OrbTone.success => (c.success, Color.lerp(c.success, c.ink, 0.35)!),
    OrbTone.warning => (c.warning, Color.lerp(c.warning, c.ink, 0.35)!),
    OrbTone.error => (c.error, Color.lerp(c.error, c.ink, 0.35)!),
    OrbTone.pending => (c.pending, Color.lerp(c.pending, c.ink, 0.35)!),
    OrbTone.ink => (Color.lerp(c.selected, Colors.white, 0.25)!, c.selected),
  };

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final soft = tone == OrbTone.soft;
    var (a, b) = _body(c);
    if (color != null && !soft) (a, b) = (color!, Color.lerp(color!, c.ink, 0.35)!);
    final glyph = soft ? (color ?? c.primary) : Colors.white;
    final shade = soft ? (color ?? c.primary) : b;
    return ExcludeSemantics(
      child: SizedBox.square(
        dimension: size,
        child: DecoratedBox(
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            gradient: RadialGradient(
              center: const Alignment(-0.38, -0.5),
              radius: 1.05,
              colors: soft
                  ? [
                      Colors.white,
                      Color.alphaBlend(shade.withValues(alpha: 0.10), b),
                      Color.alphaBlend(shade.withValues(alpha: 0.20), b),
                    ]
                  : [Color.lerp(a, Colors.white, 0.32)!, a, b],
              stops: const [0, 0.48, 1],
            ),
            border: soft ? Border.all(color: shade.withValues(alpha: 0.14)) : null,
            boxShadow: [
              BoxShadow(
                color: shade.withValues(alpha: soft ? 0.16 : 0.34),
                blurRadius: size * 0.36,
                offset: Offset(0, size * 0.16),
              ),
            ],
          ),
          child: Stack(
            alignment: Alignment.center,
            children: [
              // the specular highlight, a small soft ellipse up and to the left
              Positioned(
                left: size * 0.16,
                top: size * 0.08,
                width: size * 0.46,
                height: size * 0.30,
                child: DecoratedBox(
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(size),
                    gradient: RadialGradient(
                      colors: [
                        Colors.white.withValues(alpha: soft ? 0.9 : 0.55),
                        Colors.white.withValues(alpha: 0),
                      ],
                    ),
                  ),
                ),
              ),
              Icon(
                icon,
                size: size * 0.46,
                color: glyph,
                shadows: [
                  Shadow(
                    color: (soft ? shade : Colors.black).withValues(alpha: soft ? 0.18 : 0.28),
                    offset: Offset(0, size * 0.03),
                    blurRadius: size * 0.08,
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Floats its child gently up and down and turns it a touch, like something held in the air. Ambient: still under
/// reduced motion and in tests.
class Floating extends StatefulWidget {
  const Floating({super.key, required this.child, this.amplitude = 4, this.period = const Duration(seconds: 4)});

  final Widget child;
  final double amplitude;
  final Duration period;

  @override
  State<Floating> createState() => _FloatingState();
}

class _FloatingState extends State<Floating> with SingleTickerProviderStateMixin {
  late final _c = AnimationController(vsync: this, duration: widget.period);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final on = Ambient.enabled && !context.reduceMotion;
    if (on && !_c.isAnimating) {
      _c.repeat();
    } else if (!on && _c.isAnimating) {
      _c.stop();
    }
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _c,
    child: widget.child,
    builder: (context, child) {
      final t = _c.value * 2 * math.pi;
      return Transform(
        alignment: Alignment.center,
        transform: Matrix4.identity()
          ..setEntry(3, 2, _perspective)
          ..translateByDouble(0, math.sin(t) * widget.amplitude, 0, 1)
          ..rotateY(math.sin(t + 1.1) * 0.22)
          ..rotateX(math.cos(t) * 0.08),
        child: child,
      );
    },
  );
}

/// An icon that flips into place once when it appears (the chosen tab): a turn about its vertical axis with a small
/// overshoot. Still under reduced motion.
class FlipIn extends StatelessWidget {
  const FlipIn({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    if (context.reduceMotion) return child;
    return TweenAnimationBuilder<double>(
      tween: Tween(begin: 0, end: 1),
      duration: Motion.emphasis * 1.6,
      curve: Curves.easeOutBack,
      child: child,
      builder: (context, t, child) => Transform(
        alignment: Alignment.center,
        transform: Matrix4.identity()
          ..setEntry(3, 2, _perspective * 2)
          ..rotateY((1 - t) * math.pi)
          ..scaleByDouble(0.7 + 0.3 * t, 0.7 + 0.3 * t, 1, 1),
        child: child,
      ),
    );
  }
}

/// The fluid behind every page: the page colour with three large soft lights (coral, violet and a warm gold) drifting
/// slowly on their own paths, so the page breathes without anything to watch. Drawn in one painter; still under
/// reduced motion and in tests.
class FluidBackdrop extends StatefulWidget {
  const FluidBackdrop({super.key, this.period = const Duration(seconds: 28)});

  final Duration period;

  @override
  State<FluidBackdrop> createState() => _FluidBackdropState();
}

class _FluidBackdropState extends State<FluidBackdrop> with SingleTickerProviderStateMixin {
  late final _c = AnimationController(vsync: this, duration: widget.period);

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final on = Ambient.enabled && !context.reduceMotion;
    if (on && !_c.isAnimating) {
      _c.repeat();
    } else if (!on && _c.isAnimating) {
      _c.stop();
    }
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: IgnorePointer(
      child: RepaintBoundary(
        child: CustomPaint(painter: _FluidPainter(_c, context.colors), size: Size.infinite),
      ),
    ),
  );
}

class _FluidPainter extends CustomPainter {
  _FluidPainter(this.t, this.c) : super(repaint: t);

  final Animation<double> t;
  final AppColors c;

  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawRect(Offset.zero & size, Paint()..color = c.background);
    final a = t.value * 2 * math.pi;
    final r = math.max(size.width, 420.0);
    void light(Color color, double alpha, Offset centre, double radius) {
      final rect = Rect.fromCircle(center: centre, radius: radius);
      canvas.drawRect(
        rect,
        Paint()
          ..shader = RadialGradient(
            colors: [
              color.withValues(alpha: alpha),
              color.withValues(alpha: alpha * 0.35),
              color.withValues(alpha: 0),
            ],
            stops: const [0, 0.5, 1],
          ).createShader(rect),
      );
    }

    final w = size.width;
    final h = size.height;
    light(c.heroGlow, 0.42, Offset(w * (0.92 + 0.10 * math.sin(a)), h * (0.06 + 0.05 * math.cos(a))), r * 0.85);
    light(
      c.glowViolet,
      0.34,
      Offset(w * (0.05 + 0.12 * math.cos(a + 2)), h * (0.42 + 0.08 * math.sin(a + 2))),
      r * 0.9,
    );
    light(c.gold, 0.16, Offset(w * (0.70 + 0.14 * math.sin(a + 4)), h * (0.86 + 0.05 * math.cos(a + 4))), r * 0.7);
  }

  @override
  bool shouldRepaint(_FluidPainter old) => old.c != c;
}

/// Pages in depth: the new page rises out of the screen toward you as it fades in, while the page it covers leans
/// back, recedes and dissolves. Pages are transparent over the one [Grounded] fluid, so the two blend through it.
/// Under reduced motion pages simply cross-fade.
Widget depthTransition(
  BuildContext context,
  Animation<double> animation,
  Animation<double> secondaryAnimation,
  Widget child,
) {
  if (context.reduceMotion) {
    return FadeTransition(
      opacity: animation,
      child: FadeTransition(opacity: ReverseAnimation(secondaryAnimation), child: child),
    );
  }
  return AnimatedBuilder(
    animation: Listenable.merge([animation, secondaryAnimation]),
    child: child,
    builder: (context, child) {
      final a = animation.value;
      final o = secondaryAnimation.value;
      if (a == 1 && o == 0) return child!;
      final inn = Curves.easeOutCubic.transform(a);
      final out = Curves.easeInOutCubic.transform(o);
      final scale = (1.07 - 0.07 * inn) * (1 - 0.07 * out);
      final m = Matrix4.identity()
        ..setEntry(3, 2, _perspective)
        ..translateByDouble(0, 28 * (1 - inn), 0, 1)
        ..rotateX(-0.14 * (1 - inn) + 0.10 * out)
        ..scaleByDouble(scale, scale, 1, 1);
      return Opacity(
        opacity: (const Interval(0, 0.75).transform(a) * (1 - const Interval(0.1, 0.8).transform(o))).clamp(0.0, 1.0),
        child: Transform(alignment: Alignment.center, transform: m, child: child),
      );
    },
  );
}

/// [depthTransition] for routes pushed with Navigator rather than the router.
class DepthPageTransitionsBuilder extends PageTransitionsBuilder {
  const DepthPageTransitionsBuilder();

  @override
  Widget buildTransitions<T>(
    PageRoute<T> route,
    BuildContext context,
    Animation<double> animation,
    Animation<double> secondaryAnimation,
    Widget child,
  ) => depthTransition(context, animation, secondaryAnimation, child);
}

/// The app's ground: the [FluidBackdrop] once, under every page (scaffolds are transparent).
class Grounded extends StatelessWidget {
  const Grounded({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) => Stack(fit: StackFit.expand, children: [const FluidBackdrop(), child]);
}

/// Turns a tab's content in when the chosen tab changes: it swings up out of a slight lean and fades from half, so
/// switching tabs feels like turning to another face, not a cut. Keeps the same child (the tabs keep their state).
class TabArrival extends StatefulWidget {
  const TabArrival({super.key, required this.index, required this.child});

  final int index;
  final Widget child;

  @override
  State<TabArrival> createState() => _TabArrivalState();
}

class _TabArrivalState extends State<TabArrival> with SingleTickerProviderStateMixin {
  late final _c = AnimationController(vsync: this, duration: Motion.emphasis, value: 1);

  @override
  void didUpdateWidget(TabArrival old) {
    super.didUpdateWidget(old);
    if (old.index != widget.index && !context.reduceMotion) _c.forward(from: 0);
  }

  @override
  void dispose() {
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: _c,
    child: widget.child,
    builder: (context, child) {
      if (_c.value == 1) return child!;
      final t = Motion.settle.transform(_c.value);
      return Opacity(
        opacity: 0.4 + 0.6 * t,
        child: Transform(
          alignment: Alignment.topCenter,
          transform: Matrix4.identity()
            ..setEntry(3, 2, _perspective)
            ..rotateX(0.12 * (1 - t))
            ..scaleByDouble(0.97 + 0.03 * t, 0.97 + 0.03 * t, 1, 1),
          child: child,
        ),
      );
    },
  );
}

/// The raised, glossy face of the app's filled buttons (a theme `backgroundBuilder`): the sunset lit from above with a
/// gloss on its upper half; pressed, the light flips and the face sinks, like a key going down.
Widget raisedButtonFace(
  BuildContext context,
  Set<WidgetState> states,
  Widget? child, {
  required AppColors c,
  Color? base,
}) {
  if (states.contains(WidgetState.disabled)) return child ?? const SizedBox();
  final (from, to) = base == null ? (c.primary, c.accent) : (base, base);
  final down = states.contains(WidgetState.pressed);
  final lit = states.contains(WidgetState.hovered) || states.contains(WidgetState.focused);
  return AnimatedContainer(
    duration: Motion.quick,
    curve: Motion.enter,
    decoration: ShapeDecoration(
      shape: const StadiumBorder(),
      gradient: LinearGradient(
        begin: down ? Alignment.bottomLeft : Alignment.topLeft,
        end: down ? Alignment.topRight : Alignment.bottomRight,
        colors: [Color.lerp(from, Colors.white, lit ? 0.16 : 0.06)!, from, to],
        stops: const [0, 0.45, 1],
      ),
    ),
    foregroundDecoration: ShapeDecoration(
      shape: const StadiumBorder(),
      gradient: LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: down
            ? [Colors.black.withValues(alpha: 0.12), Colors.black.withValues(alpha: 0)]
            : [
                Colors.white.withValues(alpha: 0.26),
                Colors.white.withValues(alpha: 0.0),
                Colors.black.withValues(alpha: 0.08),
              ],
        stops: down ? const [0, 0.5] : const [0, 0.5, 1],
      ),
    ),
    child: AnimatedSlide(offset: Offset(0, down ? 0.03 : 0), duration: Motion.quick, child: child),
  );
}

/// The face of the outlined and tonal buttons: a pearl, white at the top into the muted surface, pressed darker.
Widget pearlButtonFace(BuildContext context, Set<WidgetState> states, Widget? child, {required AppColors c}) {
  if (states.contains(WidgetState.disabled)) return child ?? const SizedBox();
  final down = states.contains(WidgetState.pressed);
  return AnimatedContainer(
    duration: Motion.quick,
    decoration: ShapeDecoration(
      shape: const StadiumBorder(),
      gradient: LinearGradient(
        begin: Alignment.topCenter,
        end: Alignment.bottomCenter,
        colors: down ? [c.surfaceMuted, c.surface] : [c.surface, c.surfaceRaised, c.surfaceMuted],
        stops: down ? null : const [0, 0.55, 1],
      ),
    ),
    child: child,
  );
}
