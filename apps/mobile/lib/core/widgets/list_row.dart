import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// A list row (MASTER.md §7): an icon in a tinted disc, a title with an optional second line, and a trailing amount or
/// chevron. 64 dp tall, hairline between rows when stacked in a `RowGroup`.
class ListRow extends StatelessWidget {
  const ListRow({
    super.key,
    this.icon,
    this.iconColor,
    this.leading,
    required this.title,
    this.subtitle,
    this.trailing,
    this.trailingText,
    this.trailingColor,
    this.onTap,
    this.chevron = false,
    this.semanticsLabel,
  });

  final IconData? icon;
  final Color? iconColor;

  /// Replaces the icon disc.
  final Widget? leading;
  final String title;
  final String? subtitle;
  final Widget? trailing;
  final String? trailingText;
  final Color? trailingColor;
  final VoidCallback? onTap;
  final bool chevron;
  final String? semanticsLabel;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    final disc =
        leading ??
        (icon == null
            ? null
            : Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: (iconColor ?? c.primary).withValues(alpha: c.isDark ? 0.18 : 0.10),
                  shape: BoxShape.circle,
                ),
                child: Icon(icon, size: 20, color: iconColor ?? c.primary),
              ));
    final row = Padding(
      padding: const EdgeInsets.symmetric(horizontal: Space.lg, vertical: Space.md),
      child: Row(
        children: [
          if (disc != null) ...[ExcludeSemantics(child: disc), const SizedBox(width: Space.md)],
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: t.bodyLarge?.copyWith(fontWeight: FontWeight.w500)),
                if (subtitle != null) ...[const SizedBox(height: Space.xxs), Text(subtitle!, style: t.bodySmall)],
              ],
            ),
          ),
          if (trailingText != null) ...[
            const SizedBox(width: Space.md),
            Text(trailingText!, style: t.titleMedium?.copyWith(color: trailingColor ?? c.ink)),
          ],
          if (trailing != null) ...[const SizedBox(width: Space.md), trailing!],
          if (chevron) ...[
            const SizedBox(width: Space.sm),
            ExcludeSemantics(child: Icon(Icons.chevron_right_rounded, color: c.inkMuted)),
          ],
        ],
      ),
    );
    final content = onTap == null ? row : InkWell(onTap: onTap, child: row);
    return Semantics(
      label: semanticsLabel,
      button: onTap != null,
      excludeSemantics: semanticsLabel != null,
      child: Material(type: MaterialType.transparency, child: content),
    );
  }
}

/// Rows stacked in a card with hairlines between them.
class RowGroup extends StatelessWidget {
  const RowGroup({super.key, required this.children, this.flat = false});

  final List<Widget> children;
  final bool flat;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final radius = BorderRadius.circular(Radii.lg);
    return DecoratedBox(
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: radius,
        border: Border.all(color: c.border),
        boxShadow: flat ? null : Elevation.card(c),
      ),
      child: ClipRRect(
        borderRadius: radius,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            for (var i = 0; i < children.length; i++) ...[
              if (i > 0) Divider(indent: Space.lg, endIndent: Space.lg, color: c.border),
              children[i],
            ],
          ],
        ),
      ),
    );
  }
}

/// A small titled section: an eyebrow label and an optional action on the right.
class SectionHeader extends StatelessWidget {
  const SectionHeader({super.key, required this.title, this.action, this.onAction});

  final String title;
  final String? action;
  final VoidCallback? onAction;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.only(bottom: Space.sm),
      child: Row(
        children: [
          Expanded(
            child: Semantics(header: true, child: Text(title, style: t.titleMedium)),
          ),
          if (action != null) TextButton(onPressed: onAction, child: Text(action!)),
        ],
      ),
    );
  }
}
