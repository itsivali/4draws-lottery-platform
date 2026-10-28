import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

enum StatusTone { success, warning, error, pending, neutral }

extension StatusToneColors on StatusTone {
  (Color, IconData) resolve(AppColors c) => switch (this) {
    StatusTone.success => (c.success, Icons.check_circle_rounded),
    StatusTone.warning => (c.warning, Icons.warning_rounded),
    StatusTone.error => (c.error, Icons.error_rounded),
    StatusTone.pending => (c.pending, Icons.schedule_rounded),
    StatusTone.neutral => (c.inkMuted, Icons.circle_outlined),
  };
}

/// A state shown with an icon, a word and a colour together, never colour alone (MASTER.md §2).
class StatusPill extends StatelessWidget {
  const StatusPill({super.key, required this.tone, required this.label, this.compact = false});

  final StatusTone tone;
  final String label;

  /// Smaller, for inside list rows.
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final (color, icon) = tone.resolve(c);
    final t = Theme.of(context).textTheme;
    return Container(
      padding: compact
          ? const EdgeInsets.symmetric(horizontal: Space.sm, vertical: Space.xxs)
          : const EdgeInsets.symmetric(horizontal: Space.sm + Space.xxs, vertical: Space.xs),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(Radii.full),
        border: Border.all(color: color.withValues(alpha: 0.45)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          ExcludeSemantics(
            child: Icon(icon, size: compact ? 14 : 16, color: color),
          ),
          const SizedBox(width: Space.xs),
          Flexible(
            child: Text(label, style: (compact ? t.labelSmall : t.labelMedium)?.copyWith(color: color)),
          ),
        ],
      ),
    );
  }
}
