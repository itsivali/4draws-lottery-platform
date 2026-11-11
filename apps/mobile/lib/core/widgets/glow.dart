import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../../app/theme/theme.dart';
import '../../app/theme/tokens.dart';

// Shared shapes (MASTER.md §5, §7): the big amount, the quick actions on a hero surface, a round icon button, the ring
// gauge, the wheel's dotted dial, the plays bar chart and the segmented toggle. Every one is drawn in code from the
// theme's colours; none of them carries meaning by colour alone.

/// A sum as a big numeral: the currency small and muted, the figure large and tightly set. It is one
/// text (so it reads, and is found, as "KES 1,250") and shrinks to fit rather than wrap.
class BigAmount extends StatelessWidget {
  const BigAmount({super.key, required this.text, this.style, this.color, this.unitColor});

  /// The formatted sum, such as "KES 1,250".
  final String text;
  final TextStyle? style;
  final Color? color;

  /// The currency's colour: muted ink by default; pass the hero's muted colour on a hero surface.
  final Color? unitColor;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final big = (style ?? Theme.of(context).textTheme.displayLarge)!.copyWith(color: color ?? c.ink);
    final space = text.indexOf(' ');
    final unit = space < 0 ? '' : text.substring(0, space + 1);
    final figure = space < 0 ? text : text.substring(space + 1);
    return FittedBox(
      fit: BoxFit.scaleDown,
      alignment: AlignmentDirectional.centerStart,
      child: Text.rich(
        TextSpan(
          children: [
            if (unit.isNotEmpty)
              TextSpan(
                text: unit,
                style: big
                    .copyWith(fontSize: (big.fontSize ?? 48) * 0.38, letterSpacing: 0, color: unitColor ?? c.inkMuted)
                    .weighted(FontWeight.w500),
              ),
            TextSpan(text: figure),
          ],
        ),
        style: big,
        maxLines: 1,
      ),
    );
  }
}

/// A quick action on a hero surface: an icon over a word in a tonal tile (a light wash of the hero's own colour). 48 dp
/// tall at the least; at large text sizes it becomes a pill with the icon beside the word, so the word is never shrunk.
class GlassChip extends StatelessWidget {
  const GlassChip({super.key, required this.icon, required this.label, required this.onPressed, this.stacked = true});

  final IconData icon;
  final String label;
  final VoidCallback? onPressed;

  /// Icon over word (a tile); false for icon beside word (a pill).
  final bool stacked;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    final disc = Icon(icon, size: 24, color: c.onHero);
    final word = Text(label, textAlign: TextAlign.center);
    return _button(c, t, disc, word);
  }

  Widget _button(AppColors c, TextTheme t, Widget disc, Widget word) {
    return TextButton(
      style: TextButton.styleFrom(
        backgroundColor: c.glass,
        foregroundColor: c.onHero,
        minimumSize: const Size(Breakpoints.minTouch, Breakpoints.minTouch),
        padding: stacked
            ? const EdgeInsets.symmetric(horizontal: Space.sm, vertical: Space.md)
            : const EdgeInsets.fromLTRB(Space.sm, Space.xs, Space.lg, Space.xs),
        shape: stacked
            ? RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(Radii.lg),
                side: BorderSide(color: c.glassBorder),
              )
            : StadiumBorder(side: BorderSide(color: c.glassBorder)),
        textStyle: t.labelLarge?.copyWith(fontWeight: FontWeight.w600),
      ),
      onPressed: onPressed,
      child: stacked
          ? Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                ExcludeSemantics(child: disc),
                const SizedBox(height: Space.sm),
                word,
              ],
            )
          : Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                ExcludeSemantics(child: disc),
                const SizedBox(width: Space.sm),
                Flexible(child: word),
              ],
            ),
    );
  }
}

/// Quick actions side by side as equal tonal tiles; at large text sizes they wrap as pills instead.
class GlassChipRow extends StatelessWidget {
  const GlassChipRow({super.key, required this.chips});

  /// (icon, label, action) for each chip.
  final List<(IconData, String, VoidCallback?)> chips;

  @override
  Widget build(BuildContext context) {
    final large = MediaQuery.textScalerOf(context).scale(Breakpoints.minTouch) > Breakpoints.minTouch * 1.3;
    if (large) {
      return Wrap(
        spacing: Space.sm,
        runSpacing: Space.sm,
        children: [for (final (i, l, a) in chips) GlassChip(icon: i, label: l, onPressed: a, stacked: false)],
      );
    }
    return Row(
      children: [
        for (final (n, (i, l, a)) in chips.indexed) ...[
          if (n > 0) const SizedBox(width: Space.sm),
          Expanded(
            child: GlassChip(icon: i, label: l, onPressed: a),
          ),
        ],
      ],
    );
  }
}

/// A small round icon button (Material's tonal icon button): on a card, or on a hero surface with [dark]. Always
/// carries a tooltip, which is also its label for screen readers.
class RoundIconButton extends StatelessWidget {
  const RoundIconButton({
    super.key,
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.dark = false,
  });

  final IconData icon;
  final String tooltip;
  final VoidCallback? onPressed;

  /// On a hero surface: a light wash with a white icon.
  final bool dark;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return IconButton(
      tooltip: tooltip,
      onPressed: onPressed,
      icon: Icon(icon, size: 22),
      style: IconButton.styleFrom(
        backgroundColor: dark ? c.glass : c.selected,
        foregroundColor: dark ? c.onHero : c.onSelected,
        minimumSize: const Size.square(Breakpoints.minTouch),
        side: dark ? BorderSide(color: c.glassBorder) : null,
        shape: const CircleBorder(),
      ),
    );
  }
}

/// A small ring gauge with something (usually a number) inside it. The arc runs coral into violet over a pale track;
/// it eases to a new value, and simply shows it under reduced motion. Decorative: say the value in text beside it.
class RingGauge extends StatelessWidget {
  const RingGauge({super.key, required this.value, required this.child, this.size = 72, this.stroke = 7});

  /// 0 to 1.
  final double value;
  final Widget child;
  final double size;
  final double stroke;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return ExcludeSemantics(
      child: SizedBox.square(
        dimension: size,
        child: TweenAnimationBuilder<double>(
          tween: Tween(end: value.clamp(0, 1).toDouble()),
          duration: context.reduceMotion ? Duration.zero : Motion.emphasis,
          curve: Motion.settle,
          builder: (_, v, inner) => CustomPaint(
            painter: _RingPainter(value: v, stroke: stroke, colors: c),
            child: inner,
          ),
          child: Center(
            child: Padding(
              padding: EdgeInsets.all(stroke + Space.xxs),
              child: FittedBox(fit: BoxFit.scaleDown, child: child),
            ),
          ),
        ),
      ),
    );
  }
}

class _RingPainter extends CustomPainter {
  _RingPainter({required this.value, required this.stroke, required this.colors});

  final double value;
  final double stroke;
  final AppColors colors;

  @override
  void paint(Canvas canvas, Size size) {
    final rect = (Offset.zero & size).deflate(stroke / 2);
    final track = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..color = colors.track;
    canvas.drawArc(rect, 0, math.pi * 2, false, track);
    if (value <= 0) return;
    final sweep = math.pi * 2 * value;
    final arc = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = stroke
      ..strokeCap = StrokeCap.round
      ..shader = SweepGradient(
        startAngle: 0,
        endAngle: math.pi * 2,
        colors: [colors.primary, colors.accent, colors.primary],
        transform: const GradientRotation(-math.pi / 2),
      ).createShader(rect);
    canvas.drawArc(rect, -math.pi / 2, sweep, false, arc);
  }

  @override
  bool shouldRepaint(_RingPainter old) => old.value != value || old.colors != colors || old.stroke != stroke;
}

/// A dial of dots around whatever sits inside it (the wheel), like a watch's bezel: small dots, every fourth one
/// larger. Static: it never turns, so there is nothing to reduce.
class DottedDial extends StatelessWidget {
  const DottedDial({super.key, required this.child, this.color, this.accent});

  final Widget child;

  /// The dots' colour, and the larger dots' (default: the hero's text colour, faint and strong).
  final Color? color;
  final Color? accent;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return AspectRatio(
      aspectRatio: 1,
      child: LayoutBuilder(
        builder: (context, box) {
          final side = math.min(box.maxWidth, box.maxHeight);
          return Stack(
            fit: StackFit.expand,
            children: [
              ExcludeSemantics(
                child: CustomPaint(
                  painter: _DialPainter(
                    dot: color ?? c.onHero.withValues(alpha: 0.30),
                    major: accent ?? c.heroGlow.withValues(alpha: 0.95),
                  ),
                ),
              ),
              Padding(padding: EdgeInsets.all(side * 0.065), child: child),
            ],
          );
        },
      ),
    );
  }
}

class _DialPainter extends CustomPainter {
  _DialPainter({required this.dot, required this.major});

  final Color dot;
  final Color major;

  static const _dots = 72;

  @override
  void paint(Canvas canvas, Size size) {
    final centre = size.center(Offset.zero);
    final r = size.shortestSide / 2 * 0.975;
    final small = size.shortestSide * 0.0055;
    final big = size.shortestSide * 0.0095;
    final p = Paint()..isAntiAlias = true;
    for (var i = 0; i < _dots; i++) {
      final a = -math.pi / 2 + i * math.pi * 2 / _dots;
      final isMajor = i % 6 == 0;
      p.color = isMajor ? major : dot;
      canvas.drawCircle(centre + Offset(math.cos(a), math.sin(a)) * r, isMajor ? big : small, p);
    }
  }

  @override
  bool shouldRepaint(_DialPainter old) => old.dot != dot || old.major != major;
}

/// One bar of a [GlowBars] chart.
class GlowBar {
  const GlowBar({required this.value, required this.label, this.selected = false, this.mark});

  /// 0 to 1: how much of the bar is filled.
  final double value;

  /// The text in the round chip under the bar.
  final String label;

  /// The selected bar glows and its chip is filled.
  final bool selected;

  /// A small symbol above the bar (the lucky star), if any.
  final Widget? mark;
}

/// A vertical bar chart: pale capsule tracks, filled indigo, the selected bar glowing, with round chips
/// under the bars and the selected chip filled. Decorative to screen readers: [semanticsLabel] says what it shows.
class GlowBars extends StatelessWidget {
  const GlowBars({super.key, required this.bars, required this.semanticsLabel, this.height = 96});

  final List<GlowBar> bars;
  final String semanticsLabel;
  final double height;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    return Semantics(
      label: semanticsLabel,
      excludeSemantics: true,
      child: LayoutBuilder(
        builder: (context, box) {
          final slot = box.maxWidth / math.max(bars.length, 1);
          final barWidth = math.min(slot * 0.42, 28.0);
          final chip = math.min(slot - Space.xs, 36.0);
          final marks = bars.any((b) => b.mark != null);
          return Row(
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              for (final b in bars)
                Expanded(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (marks)
                        SizedBox(
                          height: Space.xl,
                          child: b.mark == null ? null : Center(child: b.mark),
                        ),
                      SizedBox(
                        height: height,
                        width: barWidth,
                        child: TweenAnimationBuilder<double>(
                          tween: Tween(end: b.value.clamp(0, 1).toDouble()),
                          duration: context.reduceMotion ? Duration.zero : Motion.emphasis,
                          curve: Motion.settle,
                          builder: (_, v, _) => _Bar(value: v, selected: b.selected, colors: c),
                        ),
                      ),
                      const SizedBox(height: Space.sm),
                      Container(
                        width: chip,
                        height: chip,
                        alignment: Alignment.center,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: b.selected ? c.primary : c.surface,
                          border: b.selected ? null : Border.all(color: c.border, width: 1.5),
                        ),
                        child: Padding(
                          padding: const EdgeInsets.all(Space.xs),
                          child: FittedBox(
                            fit: BoxFit.scaleDown,
                            child: Text(
                              b.label,
                              style: t.labelLarge?.copyWith(
                                fontWeight: FontWeight.w600,
                                color: b.selected ? c.onPrimary : c.ink,
                              ),
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}

class _Bar extends StatelessWidget {
  const _Bar({required this.value, required this.selected, required this.colors});

  final double value;
  final bool selected;
  final AppColors colors;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, box) {
      final c = colors;
      final radius = BorderRadius.circular(Radii.full);
      return Stack(
        clipBehavior: Clip.none,
        alignment: Alignment.bottomCenter,
        children: [
          if (selected)
            Positioned(
              left: -box.maxWidth * 1.4,
              right: -box.maxWidth * 1.4,
              top: box.maxHeight * (1 - value) - box.maxWidth,
              bottom: -box.maxWidth * 0.6,
              child: DecoratedBox(
                decoration: BoxDecoration(gradient: Gradients.blob(c.heroGlow, radius: 0.55, alpha: 0.55)),
              ),
            ),
          DecoratedBox(
            decoration: BoxDecoration(color: c.track, borderRadius: radius),
            child: const SizedBox.expand(),
          ),
          if (value > 0)
            SizedBox(
              width: box.maxWidth,
              height: math.max(box.maxHeight * value, box.maxWidth),
              child: DecoratedBox(
                decoration: BoxDecoration(
                  borderRadius: radius,
                  gradient: selected
                      ? Gradients.bar(c)
                      : LinearGradient(
                          begin: Alignment.bottomCenter,
                          end: Alignment.topCenter,
                          colors: [c.primary.withValues(alpha: 0.55), c.accent.withValues(alpha: 0.45)],
                        ),
                  boxShadow: selected ? Elevation.glow(c.primary) : null,
                ),
              ),
            ),
        ],
      );
    },
  );
}

/// A segmented toggle: a tonal track, the chosen segment a white pill with bold words, the others muted. Each
/// segment is a 48 dp button that screen readers hear as selected or not.
class PillToggle<T> extends StatelessWidget {
  const PillToggle({super.key, required this.segments, required this.selected, required this.onChanged});

  /// (value, label) for each segment.
  final List<(T, String)> segments;
  final T selected;

  /// Null disables the toggle.
  final ValueChanged<T>? onChanged;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    final duration = context.reduceMotion ? Duration.zero : Motion.standard;
    return Container(
      padding: const EdgeInsets.all(Space.xs),
      decoration: BoxDecoration(color: c.surfaceMuted, borderRadius: BorderRadius.circular(Radii.full)),
      child: Row(
        children: [
          for (final (value, label) in segments)
            Expanded(
              child: Semantics(
                button: true,
                selected: value == selected,
                inMutuallyExclusiveGroup: true,
                enabled: onChanged != null,
                child: AnimatedContainer(
                  duration: duration,
                  curve: Motion.enter,
                  decoration: BoxDecoration(
                    color: value == selected ? c.surface : c.surface.withValues(alpha: 0),
                    borderRadius: BorderRadius.circular(Radii.full),
                    boxShadow: value == selected
                        ? [
                            BoxShadow(
                              color: c.shadow.withValues(alpha: 0.12),
                              blurRadius: 6,
                              offset: const Offset(0, 2),
                            ),
                          ]
                        : null,
                  ),
                  child: Material(
                    type: MaterialType.transparency,
                    shape: const StadiumBorder(),
                    clipBehavior: Clip.antiAlias,
                    child: InkWell(
                      onTap: onChanged == null || value == selected ? null : () => onChanged!(value),
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(minHeight: Breakpoints.minTouch),
                        child: Center(
                          child: Padding(
                            padding: const EdgeInsets.symmetric(horizontal: Space.sm, vertical: Space.xs),
                            child: Text(
                              label,
                              textAlign: TextAlign.center,
                              style: t.labelLarge?.copyWith(
                                fontWeight: value == selected ? FontWeight.w700 : FontWeight.w500,
                                color: value == selected ? c.ink : c.inkMuted,
                              ),
                            ),
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
