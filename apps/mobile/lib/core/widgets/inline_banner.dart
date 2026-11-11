import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';
import 'icon_disc.dart';
import 'status_pill.dart';

/// A notice inside the page (MASTER.md §11): an icon, a title, a sentence, and one action at most. The tone is carried by
/// the icon and the words as well as the colour.
class InlineBanner extends StatelessWidget {
  const InlineBanner({
    super.key,
    required this.tone,
    required this.title,
    this.message,
    this.actionLabel,
    this.onAction,
    this.icon,
    this.live = false,
  });

  final StatusTone tone;
  final String title;
  final String? message;
  final String? actionLabel;
  final VoidCallback? onAction;
  final IconData? icon;

  /// Announced to screen readers when it appears.
  final bool live;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    final (color, defaultIcon) = switch (tone) {
      StatusTone.success => (c.success, Icons.check_circle_rounded),
      StatusTone.warning => (c.warning, Icons.warning_rounded),
      StatusTone.error => (c.error, Icons.error_rounded),
      StatusTone.pending => (c.pending, Icons.schedule_rounded),
      StatusTone.neutral => (c.inkMuted, Icons.info_rounded),
    };
    return Semantics(
      liveRegion: live,
      child: Container(
        padding: const EdgeInsets.all(Space.lg),
        decoration: BoxDecoration(
          // a white card washed with the tone, so the words read the same over any page (redesign 2026-11-11)
          color: Color.alphaBlend(color.withValues(alpha: 0.07), c.surface),
          borderRadius: BorderRadius.circular(Radii.lg),
          border: Border.all(color: color.withValues(alpha: 0.30)),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            IconDisc(icon: icon ?? defaultIcon, size: 36, color: color),
            const SizedBox(width: Space.md),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Padding(
                    padding: const EdgeInsets.only(top: Space.sm),
                    child: Text(title, style: t.titleSmall),
                  ),
                  if (message != null) ...[const SizedBox(height: Space.xs), Text(message!, style: t.bodySmall)],
                  if (actionLabel != null)
                    Padding(
                      padding: const EdgeInsets.only(top: Space.md),
                      child: OutlinedButton(
                        style: OutlinedButton.styleFrom(
                          foregroundColor: color,
                          side: BorderSide(color: color.withValues(alpha: 0.6)),
                          minimumSize: const Size(Breakpoints.minTouch, Breakpoints.minTouch),
                        ),
                        onPressed: onAction,
                        child: Text(actionLabel!),
                      ),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Nothing to show yet: an icon, one sentence, one action (MASTER.md §11).
class EmptyState extends StatelessWidget {
  const EmptyState({super.key, required this.icon, required this.title, this.message, this.actionLabel, this.onAction});

  final IconData icon;
  final String title;
  final String? message;
  final String? actionLabel;
  final VoidCallback? onAction;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: Space.xl, horizontal: Space.lg),
      child: Column(
        children: [
          IconDisc(icon: icon, size: 64),
          const SizedBox(height: Space.md),
          Text(title, style: t.titleMedium, textAlign: TextAlign.center),
          if (message != null) ...[
            const SizedBox(height: Space.xs),
            Text(message!, style: t.bodySmall, textAlign: TextAlign.center),
          ],
          if (actionLabel != null) ...[
            const SizedBox(height: Space.lg),
            FilledButton.tonal(onPressed: onAction, child: Text(actionLabel!)),
          ],
        ],
      ),
    );
  }
}
