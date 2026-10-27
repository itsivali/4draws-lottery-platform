import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// The one card: a surface with a hairline border and a soft lift (MASTER.md §5, §7). `tinted` cards sit on
/// primaryContainer for quiet emphasis; `flat` ones keep the border and drop the shadow (inside sheets and lists).
class AppCard extends StatelessWidget {
  const AppCard({
    super.key,
    this.title,
    this.trailing,
    required this.child,
    this.padding = const EdgeInsets.all(Space.lg),
    this.tinted = false,
    this.flat = false,
    this.onTap,
  });

  final String? title;
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
    final radius = BorderRadius.circular(Radii.lg);
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
    return DecoratedBox(
      decoration: BoxDecoration(
        color: tinted ? c.primaryContainer : c.surface,
        borderRadius: radius,
        border: Border.all(color: tinted ? c.primaryContainer : c.border),
        boxShadow: flat || tinted ? null : Elevation.card(c),
      ),
      child: Material(
        type: MaterialType.transparency,
        borderRadius: radius,
        clipBehavior: Clip.antiAlias,
        child: onTap == null ? body : InkWell(onTap: onTap, child: body),
      ),
    );
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
    return MergeSemantics(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: t.labelSmall),
          const SizedBox(height: Space.xxs),
          if (selectable) SelectableText(value, style: style) else Text(value, style: style),
        ],
      ),
    );
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
