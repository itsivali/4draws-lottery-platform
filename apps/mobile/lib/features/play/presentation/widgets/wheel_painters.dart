import 'dart:math';

import 'package:flutter/material.dart';

import '../../../../app/theme/tokens.dart';
import '../../domain/wheel_geometry.dart';

// The wheel is drawn in unit proportions of its radius, so it looks the same at any size and resizing never moves a
// slot. Angle convention (see WheelGeometry): wheel angle a, clockwise from 12 o'clock, is canvas angle a − π/2.

const _rimInner = 0.90;
const _studRadius = 0.86;
const _labelRadius = 0.70;
const _hubRadius = 0.24;

/// Proportions the animated layers share with the static ones (see wheel_fx_painters.dart).
const wheelRimInner = _rimInner;
const wheelHubRadius = _hubRadius;
const wheelCheckerInner = _checkerInner;
const wheelCheckerOuter = _checkerOuter;

// Each segment is two-tone (owner, 2026-10-28): a solid outer band carries the number; from the hub's fading circle out
// to _checkerOuter the segment is a checker of white and its own colour.
const _checkerInner = _hubRadius + 0.01;
const _checkerOuter = 0.52;
const _checkerCellsAcross = 3;

Offset _polar(Offset c, double r, double wheelAngle) =>
    c + Offset(cos(wheelAngle - pi / 2), sin(wheelAngle - pi / 2)) * r;

/// One square of a segment's checker band: [ring] counts outwards from the hub, [column] across the segment.
typedef CheckerCell = ({Path path, int ring, int column, bool white});

/// The checker squares of the segment starting at wheel angle [start] and [sweep] wide, on a wheel of radius [r]
/// centred on [c]: cells near-square, white where ring + column is even.
List<CheckerCell> checkerCells(Offset c, double r, double start, double sweep) {
  final inner = r * _checkerInner, outer = r * _checkerOuter;
  final cellAngle = sweep / _checkerCellsAcross;
  final cellArc = (inner + outer) / 2 * cellAngle;
  final rings = max(2, ((outer - inner) / cellArc).round());
  final ringDepth = (outer - inner) / rings;
  final cells = <CheckerCell>[];
  for (var j = 0; j < rings; j++) {
    for (var k = 0; k < _checkerCellsAcross; k++) {
      final r0 = inner + j * ringDepth, r1 = r0 + ringDepth;
      final a0 = start + k * cellAngle - pi / 2, a1 = a0 + cellAngle;
      final path = Path()
        ..moveTo(c.dx + cos(a0) * r0, c.dy + sin(a0) * r0)
        ..arcTo(Rect.fromCircle(center: c, radius: r0), a0, cellAngle, false)
        ..lineTo(c.dx + cos(a1) * r1, c.dy + sin(a1) * r1)
        ..arcTo(Rect.fromCircle(center: c, radius: r1), a1, -cellAngle, false)
        ..close();
      cells.add((path: path, ring: j, column: k, white: (j + k).isEven));
    }
  }
  return cells;
}

/// The rotating disc: segments, slot numbers, the lucky star and the boundary studs. It paints once per geometry;
/// rotation is applied by a transform above it, so spinning does not repaint it.
class WheelDiscPainter extends CustomPainter {
  WheelDiscPainter({required this.geometry, required this.colors, required this.labelled, required this.textScaler});

  final WheelGeometry geometry;
  final AppColors colors;

  /// False before the server has said how many slots the wheel has: the segments are then plain decoration.
  final bool labelled;
  final TextScaler textScaler;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.shortestSide / 2;
    final c = size.center(Offset.zero);
    final g = geometry;
    final face = Rect.fromCircle(center: c, radius: r * _rimInner);

    for (var i = 0; i < g.slots; i++) {
      final lucky = labelled && g.isLucky(i);
      final base = lucky ? colors.wheelLucky : colors.wheelSegments[i % colors.wheelSegments.length];
      final paint = Paint()
        ..shader = RadialGradient(
          colors: [Color.lerp(base, Colors.white, 0.10)!, base, Color.lerp(base, Colors.black, 0.18)!],
          stops: const [0.0, 0.62, 1.0],
        ).createShader(face);
      canvas.drawArc(face, g.segmentStart(i) - pi / 2, g.sweep, true, paint);
      _paintChecker(canvas, c, r, g.segmentStart(i), g.sweep);
    }

    // hairline separators
    final sep = Paint()
      ..color = colors.wheelRimLight.withValues(alpha: 0.28)
      ..strokeWidth = max(1.0, r * 0.006);
    for (var i = 0; i < g.slots; i++) {
      canvas.drawLine(
        _polar(c, r * (_hubRadius + 0.02), g.segmentStart(i)),
        _polar(c, r * _rimInner, g.segmentStart(i)),
        sep,
      );
    }

    // studs on the boundaries: what the pointer "clicks" past
    final stud = Paint()..color = colors.wheelRimLight;
    final studShadow = Paint()..color = colors.shadow.withValues(alpha: 0.35);
    for (var i = 0; i < g.slots; i++) {
      final p = _polar(c, r * _studRadius, g.segmentStart(i));
      canvas.drawCircle(p + Offset(0, r * 0.006), r * 0.018, studShadow);
      canvas.drawCircle(p, r * 0.016, stud);
    }

    if (labelled) _paintLabels(canvas, c, r);
  }

  /// White squares over the inner band of one segment: cells near-square, alternating with the segment's own colour,
  /// which shows through where nothing is drawn.
  void _paintChecker(Canvas canvas, Offset c, double r, double start, double sweep) {
    final white = Paint()..color = colors.wheelChecker;
    for (final cell in checkerCells(c, r, start, sweep)) {
      if (cell.white) canvas.drawPath(cell.path, white);
    }
  }

  void _paintLabels(Canvas canvas, Offset c, double r) {
    final g = geometry;
    // label size follows the segment's width so many slots still fit; the system text scale applies, within reason
    final arc = 2 * pi * r * _labelRadius / g.slots;
    final fontSize = textScaler.clamp(maxScaleFactor: 1.3).scale(min(r * 0.11, arc * 0.42));
    for (var i = 0; i < g.slots; i++) {
      final lucky = g.isLucky(i);
      final label = '$i';
      final tp = TextPainter(
        text: TextSpan(
          text: label,
          style: TextStyle(
            fontFamily: 'IBMPlexSans',
            fontSize: fontSize,
            fontWeight: FontWeight.w600,
            color: colors.wheelLabel,
            height: 1.05,
            fontFeatures: const [FontFeature.tabularFigures()],
            // labels turn with the wheel, so 6 and 9 are underlined to tell them apart upside down
            decoration: label.contains(RegExp('[69]')) ? TextDecoration.underline : null,
            decorationColor: colors.wheelLabel,
            decorationThickness: 1.6,
          ),
        ),
        textAlign: TextAlign.center,
        textDirection: TextDirection.ltr,
      )..layout();
      final angle = g.segmentCenter(i);
      canvas.save();
      canvas.translate(c.dx, c.dy);
      canvas.rotate(angle); // upright along the radius, reading from the centre outwards
      canvas.translate(0, -r * _labelRadius);
      tp.paint(canvas, Offset(-tp.width / 2, -tp.height / 2));
      if (lucky) _star(canvas, Offset(0, tp.height / 2 + fontSize * 0.5), fontSize * 0.42, colors.wheelStar);
      canvas.restore();
    }
  }

  /// A five-pointed star (drawn, not a font glyph, so it never depends on a font having it).
  static void _star(Canvas canvas, Offset centre, double radius, Color color) {
    final path = Path();
    for (var k = 0; k < 10; k++) {
      final rr = k.isEven ? radius : radius * 0.45;
      final a = -pi / 2 + k * pi / 5;
      final p = centre + Offset(cos(a), sin(a)) * rr;
      k == 0 ? path.moveTo(p.dx, p.dy) : path.lineTo(p.dx, p.dy);
    }
    canvas.drawPath(path..close(), Paint()..color = color);
  }

  @override
  bool shouldRepaint(WheelDiscPainter old) =>
      old.geometry != geometry || old.colors != colors || old.labelled != labelled || old.textScaler != textScaler;
}

/// What does not turn: the soft shadow under the wheel.
class WheelBackplatePainter extends CustomPainter {
  WheelBackplatePainter(this.colors);

  final AppColors colors;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.shortestSide / 2;
    final c = size.center(Offset.zero);
    canvas.drawCircle(
      c + Offset(0, r * 0.04),
      r * 0.99,
      Paint()
        ..color = colors.shadow.withValues(alpha: 0.22)
        ..maskFilter = MaskFilter.blur(BlurStyle.normal, r * 0.05),
    );
  }

  @override
  bool shouldRepaint(WheelBackplatePainter old) => old.colors != colors;
}

/// The fixed metal rim, the light falling on the face, the hub and the pointer. Lighting does not rotate with the
/// wheel, which is what makes the disc read as turning under it.
class WheelFramePainter extends CustomPainter {
  WheelFramePainter(this.colors);

  final AppColors colors;

  @override
  void paint(Canvas canvas, Size size) {
    final r = size.shortestSide / 2;
    final c = size.center(Offset.zero);
    final outer = Rect.fromCircle(center: c, radius: r);

    // rim: brushed metal, lit from the top left
    final rimWidth = r * (1 - _rimInner);
    canvas.drawCircle(
      c,
      r - rimWidth / 2,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = rimWidth
        ..shader = SweepGradient(
          colors: [
            colors.wheelRimLight,
            colors.wheelRimDark,
            colors.wheelRimLight,
            colors.wheelRimDark,
            colors.wheelRimLight,
          ],
          stops: const [0.0, 0.3, 0.5, 0.8, 1.0],
          transform: const GradientRotation(-pi / 3),
        ).createShader(outer),
    );
    // bevels
    canvas.drawCircle(
      c,
      r - 0.5,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = max(1.0, r * 0.008)
        ..color = colors.wheelRimDark,
    );
    canvas.drawCircle(
      c,
      r * _rimInner,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = max(1.0, r * 0.01)
        ..color = colors.shadow.withValues(alpha: 0.45),
    );

    // sheen across the face
    canvas.drawCircle(
      c,
      r * _rimInner,
      Paint()
        ..shader = LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            Colors.white.withValues(alpha: 0.16),
            Colors.white.withValues(alpha: 0.0),
            colors.shadow.withValues(alpha: 0.12),
          ],
          stops: const [0.0, 0.45, 1.0],
        ).createShader(outer),
    );

    // hub
    final hub = Rect.fromCircle(center: c, radius: r * _hubRadius);
    canvas.drawCircle(
      c + Offset(0, r * 0.012),
      r * _hubRadius,
      Paint()
        ..color = colors.shadow.withValues(alpha: 0.35)
        ..maskFilter = MaskFilter.blur(BlurStyle.normal, r * 0.02),
    );
    canvas.drawCircle(
      c,
      r * _hubRadius,
      Paint()
        ..shader = RadialGradient(
          center: const Alignment(-0.35, -0.4),
          colors: [colors.wheelRimLight, colors.wheelRimDark],
        ).createShader(hub),
    );
    canvas.drawCircle(
      c,
      r * (_hubRadius - 0.035),
      Paint()
        ..shader = RadialGradient(
          center: const Alignment(-0.3, -0.35),
          colors: [
            Color.lerp(colors.wheelSegments.first, Colors.white, 0.12)!,
            colors.wheelSegments[colors.wheelSegments.length ~/ 2],
          ],
        ).createShader(hub),
    );
  }

  @override
  bool shouldRepaint(WheelFramePainter old) => old.colors != colors;
}
