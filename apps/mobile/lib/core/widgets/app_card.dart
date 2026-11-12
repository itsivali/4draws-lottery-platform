import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';
import 'icon_disc.dart';
import 'motion.dart';

/// The one card: a white surface with large corners that stands off the page on its shadow (MASTER.md §5, §7). `tinted` cards sit
/// on primaryContainer for quiet emphasis; `flat` ones trade the shadow for a hairline (inside sheets and lists).
class AppCard extends StatelessWidget {
  const AppCard({
    super.key,
    this.title,
    this.icon,
    this.trailing,
    required this.child,
    this.padding = const EdgeInsets.all(Space.xl),
    this.tinted = false,
    this.flat = false,
    this.onTap,
  });

  final String? title;

  /// An icon in a tinted disc before the title.
  final IconData? icon;
  final Widget? trailing;
  final Widget child;
  final EdgeInsets padding;
  final bool tinted;
  final bool flat;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    final radius = BorderRadius.circular(Radii.card);
    final body = Padding(
      padding: padding,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (title != null) ...[
            // a list card drops its side padding for its rows; the title keeps the card's inset
            Padding(
              padding: EdgeInsets.symmetric(horizontal: padding.horizontal == 0 ? Space.lg : 0),
              child: Row(
                children: [
                  if (icon != null) ...[IconDisc(icon: icon!), const SizedBox(width: Space.md)],
                  Expanded(
                    child: Semantics(header: true, child: Text(title!, style: t.titleMedium)),
                  ),
                  ?trailing,
                ],
              ),
            ),
            const SizedBox(height: Space.md),
          ],
          child,
        ],
      ),
    );
    final surface = DecoratedBox(
      decoration: BoxDecoration(
        color: tinted ? c.primaryContainer : c.surface,
        borderRadius: radius,
        border: flat ? Border.all(color: c.border) : null,
      ),
      child: Material(
        type: MaterialType.transparency,
        borderRadius: radius,
        clipBehavior: Clip.antiAlias,
        child: onTap == null ? body : InkWell(onTap: onTap, child: body),
      ),
    );
    // raised cards stand off the page on their shadow; tappable ones come forward under the pointer (Lift)
    return flat || tinted ? surface : Lift(enabled: onTap != null, child: surface);
  }
}

/// A row of label over value, the shape of every fact the app states (a price, a date, a hash).
class FactRow extends StatelessWidget {
  const FactRow({super.key, required this.label, required this.value, this.mono = false, this.selectable = false});

  final String label;
  final String value;
  final bool mono;
  final bool selectable;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final style = mono ? t.bodySmall?.copyWith(fontFamily: AppType.mono, color: context.colors.ink) : t.titleMedium;
    final fact = MergeSemantics(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Text(label, style: t.labelSmall),
          const SizedBox(height: Space.xxs),
          if (selectable) SelectableText(value, style: style) else Text(value, style: style),
        ],
      ),
    );
    // a value that can be selected is something to touch, so it is at least 48 dp tall (Play's pre-launch report
    // flags smaller touch targets)
    return selectable
        ? ConstrainedBox(
            constraints: const BoxConstraints(minHeight: Breakpoints.minTouch),
            child: fact,
          )
        : fact;
  }
}

/// Facts side by side on wide cards, stacked on narrow ones.
class FactGrid extends StatelessWidget {
  const FactGrid({super.key, required this.facts, this.columns = 2});

  final List<Widget> facts;
  final int columns;

  @override
  Widget build(BuildContext context) => LayoutBuilder(
    builder: (context, box) {
      final cols = box.maxWidth < 360 ? 1 : columns;
      final width = (box.maxWidth - (cols - 1) * Space.lg) / cols;
      return Wrap(
        spacing: Space.lg,
        runSpacing: Space.lg,
        children: [for (final f in facts) SizedBox(width: width, child: f)],
      );
    },
  );
}
