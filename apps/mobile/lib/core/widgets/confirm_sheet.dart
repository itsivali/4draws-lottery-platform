import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';
import 'content_frame.dart';
import 'depth.dart';

/// Asks before anything that moves money or limits play: what it costs, what you get, what happens next
/// (MASTER.md §7). A bottom sheet on phones, a dialog on wider screens. Returns true only on an explicit confirm.
Future<bool> confirmAction(
  BuildContext context, {
  required String title,
  required List<(String, String)> facts,
  String? note,
  required String confirmLabel,
  bool destructive = false,
}) async {
  Widget body(BuildContext ctx) =>
      _ConfirmBody(title: title, facts: facts, note: note, confirmLabel: confirmLabel, destructive: destructive);
  final bool? ok = widthClassOf(context) == WidthClass.compact
      ? await showModalBottomSheet<bool>(context: context, isScrollControlled: true, useSafeArea: true, builder: body)
      : await showDialog<bool>(
          context: context,
          builder: (ctx) => Dialog(
            child: ConstrainedBox(constraints: const BoxConstraints(maxWidth: 480), child: body(ctx)),
          ),
        );
  return ok ?? false;
}

class _ConfirmBody extends StatelessWidget {
  const _ConfirmBody({
    required this.title,
    required this.facts,
    required this.note,
    required this.confirmLabel,
    required this.destructive,
  });

  final String title;
  final List<(String, String)> facts;
  final String? note;
  final String confirmLabel;
  final bool destructive;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(Space.xl, Space.sm, Space.xl, Space.xl),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Semantics(header: true, focused: true, child: Text(title, style: t.titleLarge)),
          const SizedBox(height: Space.lg),
          for (final (label, value) in facts)
            Padding(
              padding: const EdgeInsets.only(bottom: Space.md),
              child: MergeSemantics(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(label, style: t.bodySmall),
                    const SizedBox(height: Space.xxs),
                    Text(value, style: t.titleMedium),
                  ],
                ),
              ),
            ),
          if (note != null) ...[const SizedBox(height: Space.sm), Text(note!, style: t.bodySmall)],
          const SizedBox(height: Space.xl),
          FilledButton(
            style: destructive
                ? FilledButton.styleFrom(
                    backgroundColor: c.error,
                    foregroundColor: c.onPrimary,
                    backgroundBuilder: (context, states, child) =>
                        raisedButtonFace(context, states, child, c: c, base: c.error),
                  )
                : null,
            onPressed: () => Navigator.of(context).pop(true),
            child: Text(confirmLabel),
          ),
          const SizedBox(height: Space.sm),
          TextButton(onPressed: () => Navigator.of(context).pop(false), child: const Text('Cancel')),
        ],
      ),
    );
  }
}
