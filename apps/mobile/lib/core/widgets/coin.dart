import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';
import 'motion.dart';

/// A gold coin in 3D, turning slowly about its upright axis (redesign, second pass): two struck faces (the brand's "4"
/// on the front, a star on the back) and a milled edge built from thin layers, drawn in perspective and sorted by
/// depth each frame, so its thickness shows as it turns. Light glances across the face with the turn. It stands for
/// money (the balance), never for a win. Decorative; at rest (reduced motion, tests) it holds a three-quarter pose
/// that shows its edge.
class Coin3D extends StatefulWidget {
  const Coin3D({super.key, this.size = 72, this.period = const Duration(seconds: 7)});

  final double size;

  /// One full turn.
  final Duration period;

  @override
  State<Coin3D> createState() => _Coin3DState();
}

class _Coin3DState extends State<Coin3D> with SingleTickerProviderStateMixin {
  late final _c = AnimationController(vsync: this, duration: widget.period);

  /// The pose it holds when still: turned enough to show the edge.
  static const _rest = -0.42;

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
  Widget build(BuildContext context) {
    final colors = context.colors;
    final s = widget.size;
    return ExcludeSemantics(
      child: SizedBox(
        width: s * 1.15,
        height: s * 1.3,
        child: AnimatedBuilder(
          animation: _c,
          builder: (context, _) {
            final t = _c.value;
            // it lingers face-on and sweeps through edge-on, as a turned coin does
            final turn = _rest + 2 * math.pi * t - 0.45 * math.sin(4 * math.pi * t);
            final bob = math.sin(2 * math.pi * t) * s * 0.04;
            return Transform.translate(
              offset: Offset(0, bob),
              child: Stack(
                alignment: Alignment.center,
                children: [
                  Positioned(
                    bottom: 0,
                    child: _Shadow(width: s * (0.45 + 0.4 * math.cos(turn).abs()), colors: colors),
                  ),
                  _Coin(turn: turn, size: s, colors: colors),
                ],
              ),
            );
          },
        ),
      ),
    );
  }
}

class _Shadow extends StatelessWidget {
  const _Shadow({required this.width, required this.colors});

  final double width;
  final AppColors colors;

  @override
  Widget build(BuildContext context) => Container(
    width: width,
    height: width * 0.18,
    decoration: BoxDecoration(
      borderRadius: BorderRadius.all(Radius.elliptical(width, width * 0.18)),
      color: colors.shadow.withValues(alpha: 0.14),
      boxShadow: [BoxShadow(color: colors.shadow.withValues(alpha: 0.22), blurRadius: width * 0.12)],
    ),
  );
}

/// The coin itself: every layer under one turn, the far ones painted first.
class _Coin extends StatelessWidget {
  const _Coin({required this.turn, required this.size, required this.colors});

  final double turn;
  final double size;
  final AppColors colors;

  static const _tilt = 0.22; // seen a little from above
  static const _layers = 9;

  @override
  Widget build(BuildContext context) {
    final thick = size * 0.13;
    // z grows away from the eye; each layer's depth after the turn decides the painting order
    final layers = <(double, Widget)>[];
    Matrix4 at(double z, {bool flip = false}) {
      final m = Matrix4.identity()
        ..setEntry(3, 2, 0.0016)
        ..rotateX(_tilt)
        ..rotateY(turn)
        ..translateByDouble(0, 0, z, 1);
      if (flip) m.rotateY(math.pi);
      return m;
    }

    double depth(double z) => z * math.cos(turn) * math.cos(_tilt);
    for (var i = 0; i < _layers; i++) {
      final z = -thick / 2 + thick * (i + 0.5) / _layers;
      layers.add((
        depth(z),
        Transform(
          alignment: Alignment.center,
          transform: at(z),
          child: CustomPaint(
            size: Size.square(size),
            painter: _EdgePainter(colors, ridge: i.isEven),
          ),
        ),
      ));
    }
    layers
      ..add((
        depth(-thick / 2),
        Transform(
          alignment: Alignment.center,
          transform: at(-thick / 2),
          child: CustomPaint(
            size: Size.square(size),
            painter: _FacePainter(colors, front: true, turn: turn),
          ),
        ),
      ))
      ..add((
        depth(thick / 2),
        Transform(
          alignment: Alignment.center,
          transform: at(thick / 2, flip: true),
          child: CustomPaint(
            size: Size.square(size),
            painter: _FacePainter(colors, front: false, turn: turn + math.pi),
          ),
        ),
      ));
    layers.sort((a, b) => b.$1.compareTo(a.$1));
    return SizedBox.square(
      dimension: size,
      child: Stack(children: [for (final (_, w) in layers) w]),
    );
  }
}

/// One slice of the milled edge: a darker gold disc, alternately ridged.
class _EdgePainter extends CustomPainter {
  _EdgePainter(this.c, {required this.ridge});

  final AppColors c;
  final bool ridge;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.shortestSide / 2;
    canvas.drawCircle(
      size.center(Offset.zero),
      r * 0.985,
      Paint()
        ..color = ridge
            ? Color.lerp(c.wheelRimDark, c.wheelRimLight, 0.3)!
            : Color.lerp(c.wheelRimDark, Colors.black, 0.12)!,
    );
  }

  @override
  bool shouldRepaint(_EdgePainter old) => old.ridge != ridge || old.c != c;
}

/// A struck face: a raised rim, a ring of beads, and the device (the "T" or a star) embossed in the middle, with a
/// band of light that glances across as the coin turns.
class _FacePainter extends CustomPainter {
  _FacePainter(this.c, {required this.front, required this.turn});

  final AppColors c;
  final bool front;
  final double turn;

  @override
  void paint(Canvas canvas, Size size) {
    final o = size.center(Offset.zero);
    final r = size.shortestSide / 2;
    final face = Rect.fromCircle(center: o, radius: r);
    // the bright rim gold of the wheel, not the darker text gold
    final light = Color.lerp(c.wheelRimLight, Colors.white, 0.35)!;
    final mid = Color.lerp(c.wheelRimLight, c.wheelRimDark, 0.4)!;
    final deep = c.wheelRimDark;

    canvas.drawCircle(
      o,
      r,
      Paint()
        ..shader = RadialGradient(
          center: const Alignment(-0.35, -0.45),
          radius: 1.1,
          colors: [light, mid, deep],
          stops: const [0, 0.5, 1],
        ).createShader(face),
    );
    // the raised rim and the field inside it
    canvas.drawCircle(
      o,
      r * 0.86,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = r * 0.05
        ..color = deep.withValues(alpha: 0.45),
    );
    canvas.drawCircle(
      o,
      r * 0.8,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color.lerp(c.wheelRimLight, Colors.white, 0.15)!, Color.lerp(mid, deep, 0.25)!],
        ).createShader(face),
    );
    // a ring of beads
    final bead = Paint()..color = deep.withValues(alpha: 0.35);
    for (var i = 0; i < 36; i++) {
      final a = i * 2 * math.pi / 36;
      canvas.drawCircle(o + Offset(math.cos(a), math.sin(a)) * r * 0.72, r * 0.022, bead);
    }
    // the device, embossed: a dark offset beneath, a light one above, the gold between
    final device = front ? _letter(r) : _star(o, r * 0.36);
    void draw(Offset shift, Color color) {
      canvas.save();
      canvas.translate(shift.dx, shift.dy);
      if (device case final Path p) {
        canvas.drawPath(p, Paint()..color = color);
      } else if (device case final TextPainter tp) {
        tp.text = TextSpan(text: '4', style: _style(r, color));
        tp.layout();
        tp.paint(canvas, o - Offset(tp.width / 2, tp.height / 2));
      }
      canvas.restore();
    }

    draw(Offset(r * 0.025, r * 0.035), deep.withValues(alpha: 0.75));
    draw(Offset(-r * 0.02, -r * 0.025), Colors.white.withValues(alpha: 0.7));
    draw(Offset.zero, Color.lerp(mid, deep, 0.2)!);
    if (device is TextPainter) device.dispose();

    // the glance of light: a band that crosses the face as it turns
    final x = math.sin(turn);
    canvas.save();
    canvas.clipPath(Path()..addOval(face));
    canvas.drawRect(
      face,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment(-1 + x, -1),
          end: Alignment(1 + x, 1),
          colors: [
            Colors.white.withValues(alpha: 0),
            Colors.white.withValues(alpha: 0.38),
            Colors.white.withValues(alpha: 0),
          ],
          stops: const [0.35, 0.5, 0.65],
        ).createShader(face),
    );
    canvas.restore();
  }

  TextStyle _style(double r, Color color) => TextStyle(
    fontFamily: AppType.display,
    fontVariations: const [FontVariation.weight(800)],
    fontWeight: FontWeight.w800,
    fontSize: r * 0.95,
    height: 1,
    color: color,
  );

  Object _letter(double r) => TextPainter(textDirection: TextDirection.ltr);

  Path _star(Offset o, double radius) {
    final p = Path();
    for (var k = 0; k < 10; k++) {
      final rr = k.isEven ? radius : radius * 0.45;
      final a = -math.pi / 2 + k * math.pi / 5;
      final v = o + Offset(math.cos(a), math.sin(a)) * rr;
      k == 0 ? p.moveTo(v.dx, v.dy) : p.lineTo(v.dx, v.dy);
    }
    return p..close();
  }

  @override
  bool shouldRepaint(_FacePainter old) => old.turn != turn || old.front != front || old.c != c;
}
