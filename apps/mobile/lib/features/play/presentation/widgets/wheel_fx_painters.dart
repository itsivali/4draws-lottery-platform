import 'dart:math';

import 'package:flutter/material.dart';

import '../../../../app/theme/tokens.dart';
import '../../domain/wheel_geometry.dart';
import 'wheel_painters.dart';

// The wheel's animated layers (owner, 2026-10-30: more animation, above all on the checker band). They decorate only:
// nothing here decides or moves the outcome, which stays with WheelMotion and the server's slot. Every layer reads
// one WheelFx at paint time, so animating repaints these thin layers and never the numbered disc.

/// What the animated layers draw from, updated by the wheel each frame.
class WheelFx extends ChangeNotifier {
  /// Ambient phase, 0 → 1 repeating: drives the shimmer, the twinkle and the rim lights.
  double phase = 0;

  /// How hard the wheel is spinning, 0 (at rest) → 1 (full speed): the lights quicken and brighten with it.
  double energy = 0;

  /// The wheel's rotation, for the pointer's flick as studs pass.
  double rotation = 0;

  /// The landing celebration, 0 → 1 once per outcome (0 when there is none).
  double celebrate = 0;

  /// The slot the celebration lights up.
  int? landedSlot;

  void update({
    double? phase,
    double? energy,
    double? rotation,
    double? celebrate,
    int? landedSlot,
    bool clearSlot = false,
  }) {
    if (phase != null) this.phase = phase;
    if (energy != null) this.energy = energy;
    if (rotation != null) this.rotation = rotation;
    if (celebrate != null) this.celebrate = celebrate;
    if (landedSlot != null || clearSlot) this.landedSlot = landedSlot;
    notifyListeners();
  }
}

/// Over the rotating disc (it turns with it): a light that sweeps round the checker band, squares that twinkle, and
/// the landed segment's glow.
class WheelDiscFxPainter extends CustomPainter {
  WheelDiscFxPainter({required this.fx, required this.geometry, required this.colors}) : super(repaint: fx);

  final WheelFx fx;
  final WheelGeometry geometry;
  final AppColors colors;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.shortestSide / 2;
    final c = size.center(Offset.zero);
    final g = geometry;
    final e = fx.energy.clamp(0.0, 1.0);
    final band = Path()
      ..fillType = PathFillType.evenOdd
      ..addOval(Rect.fromCircle(center: c, radius: r * wheelCheckerOuter))
      ..addOval(Rect.fromCircle(center: c, radius: r * wheelCheckerInner));

    // a comet of light running round the checker band, faster and brighter as the wheel speeds up (and against it)
    final head = -2 * pi * fx.phase * (1 + 3 * e);
    canvas.save();
    canvas.clipPath(band);
    canvas.drawCircle(
      c,
      r * wheelCheckerOuter,
      Paint()
        ..shader = SweepGradient(
          colors: [
            Colors.white.withValues(alpha: 0),
            Colors.white.withValues(alpha: 0),
            Colors.white.withValues(alpha: 0.22 + 0.33 * e),
            Colors.white.withValues(alpha: 0),
          ],
          stops: const [0, 0.72, 0.97, 1],
          transform: GradientRotation(head),
        ).createShader(Rect.fromCircle(center: c, radius: r)),
    );
    canvas.restore();

    // squares twinkle in a ripple outwards from the hub; while spinning they flash gold
    final twinkle = Paint();
    for (var i = 0; i < g.slots; i++) {
      for (final cell in checkerCells(c, r, g.segmentStart(i), g.sweep)) {
        if (!cell.white) continue;
        final wave = sin(2 * pi * (fx.phase * 2 - cell.ring * 0.18 - i / g.slots) + cell.column * 1.3);
        final a = pow(max(0.0, wave), 6) * (0.28 + 0.45 * e);
        if (a < 0.02) continue;
        twinkle.color = Color.lerp(Colors.white, colors.gold, e)!.withValues(alpha: a.toDouble());
        canvas.drawPath(cell.path, twinkle);
      }
    }

    // the landed segment breathes gold twice, then lets go
    final slot = fx.landedSlot;
    if (slot != null && fx.celebrate > 0 && fx.celebrate < 1 && slot < g.slots) {
      final pulse = sin(pi * fx.celebrate * 2).abs() * (1 - fx.celebrate * 0.4);
      final face = Rect.fromCircle(center: c, radius: r * wheelRimInner);
      final start = g.segmentStart(slot) - pi / 2;
      canvas.drawArc(face, start, g.sweep, true, Paint()..color = Colors.white.withValues(alpha: 0.28 * pulse));
      canvas.drawArc(
        face.deflate(r * 0.01),
        start,
        g.sweep,
        true,
        Paint()
          ..style = PaintingStyle.stroke
          ..strokeWidth = r * 0.025
          ..color = colors.gold.withValues(alpha: 0.9 * pulse)
          ..maskFilter = MaskFilter.blur(BlurStyle.normal, r * 0.012),
      );
    }
  }

  @override
  bool shouldRepaint(WheelDiscFxPainter old) => old.fx != fx || old.geometry != geometry || old.colors != colors;
}

/// On the fixed frame: marquee bulbs round the rim that chase, and a highlight that turns over the hub.
class WheelFrameFxPainter extends CustomPainter {
  WheelFrameFxPainter({required this.fx, required this.colors}) : super(repaint: fx);

  final WheelFx fx;
  final AppColors colors;

  static const bulbs = 24;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.shortestSide / 2;
    final c = size.center(Offset.zero);
    final e = fx.energy.clamp(0.0, 1.0);
    final ring = r * (1 + wheelRimInner) / 2;
    final chase = fx.phase * (2 + 10 * e); // bulbs per phase: a lazy drift at rest, a race while spinning
    final celebrating = fx.celebrate > 0 && fx.celebrate < 1;
    final glow = Paint()..maskFilter = MaskFilter.blur(BlurStyle.normal, r * 0.018);
    final bulb = Paint();
    for (var i = 0; i < bulbs; i++) {
      final a = 2 * pi * i / bulbs;
      final p = c + Offset(cos(a - pi / 2), sin(a - pi / 2)) * ring;
      // every third bulb lit, the pattern running round; all of them flash together on a landing
      var lit = 0.5 + 0.5 * cos(2 * pi * (i / 3 - chase));
      if (celebrating) lit = max(lit, 0.5 + 0.5 * cos(2 * pi * fx.celebrate * 4));
      final on = Color.lerp(Color.lerp(colors.wheelRimDark, Colors.black, 0.25)!, const Color(0xFFFFF3C4), lit)!;
      glow.color = const Color(0xFFFFE08A).withValues(alpha: 0.85 * lit * (0.65 + 0.35 * e));
      canvas.drawCircle(p, r * 0.042, glow);
      bulb.color = on;
      canvas.drawCircle(p, r * 0.024, bulb);
      canvas.drawCircle(
        p - Offset(r * 0.005, r * 0.005),
        r * 0.006,
        Paint()..color = Colors.white.withValues(alpha: 0.4 + 0.5 * lit),
      );
    }

    // a glint turning over the hub
    final hub = Rect.fromCircle(center: c, radius: r * wheelHubRadius);
    canvas.save();
    canvas.clipPath(Path()..addOval(hub));
    canvas.drawCircle(
      c,
      r * wheelHubRadius,
      Paint()
        ..shader = SweepGradient(
          colors: [
            Colors.white.withValues(alpha: 0),
            Colors.white.withValues(alpha: 0.28 + 0.2 * e),
            Colors.white.withValues(alpha: 0),
          ],
          stops: const [0.0, 0.08, 0.2],
          transform: GradientRotation(2 * pi * fx.phase),
        ).createShader(hub),
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(WheelFrameFxPainter old) => old.fx != fx || old.colors != colors;
}

/// The pointer at 12 o'clock. It flicks aside as each stud passes under it, more the faster the wheel turns.
class WheelPointerPainter extends CustomPainter {
  WheelPointerPainter({required this.fx, required this.colors, required this.geometry}) : super(repaint: fx);

  final WheelFx fx;
  final AppColors colors;
  final WheelGeometry geometry;

  /// The flick, in radians, for a wheel at [rotation] turning with [energy] (0 at rest).
  static double kick({required double rotation, required double energy, required double sweep}) {
    if (energy <= 0) return 0;
    final under = normalizeAngle(-rotation) / sweep; // segments past the pointer
    final sincePassed = under - under.floorToDouble(); // 0 just as a boundary stud passes
    final fromStud = min(sincePassed, 1 - sincePassed);
    return -0.32 * energy.clamp(0.0, 1.0) * max(0.0, 1 - fromStud / 0.3);
  }

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.shortestSide / 2;
    final c = size.center(Offset.zero);
    final tip = Offset(c.dx, c.dy - r * (wheelRimInner - 0.07));
    final w = r * 0.085;
    final top = c.dy - r - r * 0.05;
    final pivot = Offset(c.dx, top + r * 0.045);
    canvas.save();
    canvas.translate(pivot.dx, pivot.dy);
    canvas.rotate(kick(rotation: fx.rotation, energy: fx.energy, sweep: geometry.sweep));
    canvas.translate(-pivot.dx, -pivot.dy);
    final pointer = Path()
      ..moveTo(tip.dx, tip.dy)
      ..quadraticBezierTo(c.dx - w * 0.55, top + r * 0.12, c.dx - w, top + r * 0.035)
      ..arcToPoint(Offset(c.dx + w, top + r * 0.035), radius: Radius.circular(w * 1.05))
      ..quadraticBezierTo(c.dx + w * 0.55, top + r * 0.12, tip.dx, tip.dy)
      ..close();
    canvas.drawShadow(pointer, colors.shadow, r * 0.025, false);
    canvas.drawPath(
      pointer,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.centerLeft,
          end: Alignment.centerRight,
          colors: [colors.wheelRimLight, colors.gold, colors.wheelRimDark],
        ).createShader(Rect.fromLTRB(c.dx - w, top, c.dx + w, tip.dy)),
    );
    canvas.drawCircle(pivot, w * 0.32, Paint()..color = colors.wheelRimDark);
    canvas.restore();
  }

  @override
  bool shouldRepaint(WheelPointerPainter old) => old.fx != fx || old.colors != colors || old.geometry != geometry;
}
