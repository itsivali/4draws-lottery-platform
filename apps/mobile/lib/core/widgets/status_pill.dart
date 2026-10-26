import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

enum StatusTone { success, warning, error, pending, neutral }

/// A state shown with an icon, a word and a colour together, never colour alone (MASTER.md §2).
class StatusPill extends StatelessWidget {
  const StatusPill({super.key, required this.tone, required this.label});

  final StatusTone tone;
  final String label;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final (color, icon) = switch (tone) {
      StatusTone.success => (c.success, Icons.check_circle_rounded),
      StatusTone.warning => (c.warning, Icons.warning_rounded),
      StatusTone.error => (c.error, Icons.error_rounded),
      StatusTone.pending => (c.pending, Icons.schedule_rounded),
      StatusTone.neutral => (c.inkMuted, Icons.circle_outlined),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: Space.sm + Space.xxs, vertical: Space.xs),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.10),
        borderRadius: BorderRadius.circular(Radii.full),
        border: Border.all(color: color.withValues(alpha: 0.45)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          ExcludeSemantics(child: Icon(icon, size: 16, color: color)),
          const SizedBox(width: Space.xs),
          Flexible(
            child: Text(label, style: Theme.of(context).textTheme.labelMedium?.copyWith(color: color)),
          ),
        ],
      ),
    );
  }
}
