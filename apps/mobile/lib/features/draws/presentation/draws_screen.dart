import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/list_row.dart';
import '../../../core/widgets/motion.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/draws_providers.dart';
import '../domain/draw_models.dart';
import 'draw_copy.dart';

class DrawsScreen extends ConsumerStatefulWidget {
  const DrawsScreen({super.key});

  @override
  ConsumerState<DrawsScreen> createState() => _DrawsScreenState();
}

class _DrawsScreenState extends ConsumerState<DrawsScreen> {
  DrawType? _type;

  @override
  Widget build(BuildContext context) {
    final draws = ref.watch(drawsProvider);
    final t = Theme.of(context).textTheme;
    return PageScaffold(
      title: 'Draws',
      onRefresh: () {
        ref.invalidate(myPeriodsProvider);
        return ref.refresh(drawsProvider.future);
      },
      children: [
        Text(
          "Every chance your plays earn on a day is one entry in that day's draw and in your own weekly, monthly and "
          'quarterly draws. Winners are drawn with a public randomness beacon and approved by two officials.',
          style: t.bodyMedium?.copyWith(color: context.colors.inkMuted),
        ),
        const MyPeriodsCard(),
        Wrap(
          spacing: Space.sm,
          runSpacing: Space.sm,
          children: [
            ChoiceChip(
              label: const Text('All'),
              selected: _type == null,
              onSelected: (_) => setState(() => _type = null),
            ),
            for (final dt in DrawType.values)
              ChoiceChip(
                label: Text(drawTypeLabel(dt).replaceFirst(' draw', '')),
                selected: _type == dt,
                onSelected: (_) => setState(() => _type = dt),
              ),
          ],
        ),
        AsyncView(
          value: draws,
          what: 'the draws',
          onRetry: () => ref.invalidate(drawsProvider),
          data: (list) {
            final shown = [
              for (final d in list)
                if (_type == null || d.type == _type) d,
            ];
            if (shown.isEmpty) {
              return const EmptyState(
                icon: Icons.event_outlined,
                title: 'No draws to show yet.',
                message: 'Draws appear here as their periods open.',
              );
            }
            return Staggered(
              spacing: Space.md,
              children: [for (final d in shown) DrawTile(draw: d)],
            );
          },
        ),
      ],
    );
  }
}

class DrawTile extends StatelessWidget {
  const DrawTile({super.key, required this.draw});

  final Draw draw;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final (tone, label) = drawStatus(draw.status);
    final open = draw.status == DrawStatus.open;
    return PressScale(
      child: AppCard(
        onTap: () => context.go('/draws/${draw.id}'),
        child: Row(
          children: [
            Container(
              width: 44,
              height: 44,
              decoration: BoxDecoration(
                color: open ? c.primaryContainer : c.surfaceMuted,
                borderRadius: BorderRadius.circular(Radii.md),
              ),
              child: ExcludeSemantics(child: Icon(_icon(draw.type), color: open ? c.onPrimaryContainer : c.inkMuted)),
            ),
            const SizedBox(width: Space.md),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(drawTypeLabel(draw.type), style: t.titleMedium),
                  Text(formatPeriod(draw.periodStart, draw.periodEnd), style: t.bodySmall),
                  const SizedBox(height: Space.sm),
                  Wrap(
                    spacing: Space.sm,
                    runSpacing: Space.xs,
                    crossAxisAlignment: WrapCrossAlignment.center,
                    children: [
                      StatusPill(tone: tone, label: label, compact: true),
                      if (!open) Text('${draw.entries} entries', style: t.labelSmall),
                    ],
                  ),
                ],
              ),
            ),
            ExcludeSemantics(child: Icon(Icons.chevron_right_rounded, color: c.inkMuted)),
          ],
        ),
      ),
    );
  }

  static IconData _icon(DrawType t) => switch (t) {
    DrawType.daily => Icons.today_rounded,
    DrawType.weekly => Icons.date_range_rounded,
    DrawType.monthly => Icons.calendar_month_rounded,
    DrawType.quarterly => Icons.event_repeat_rounded,
  };
}

/// The open draws, for Home.
class OpenDrawsCard extends ConsumerWidget {
  const OpenDrawsCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final draws = ref.watch(drawsProvider);
    final c = context.colors;
    return AppCard(
      title: 'Open draws',
      trailing: TextButton(onPressed: () => context.go('/draws'), child: const Text('All draws')),
      padding: const EdgeInsets.only(top: Space.lg),
      child: AsyncView(
        value: draws,
        what: 'the draws',
        onRetry: () => ref.invalidate(drawsProvider),
        data: (list) {
          final open = list.where((d) => d.status == DrawStatus.open).take(4).toList();
          if (open.isEmpty) {
            return const EmptyState(icon: Icons.event_busy_rounded, title: 'No draw is taking entries right now.');
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final (i, d) in open.indexed) ...[
                if (i > 0) Divider(indent: Space.lg, endIndent: Space.lg, color: c.border),
                ListRow(
                  icon: DrawTile._icon(d.type),
                  title: drawTypeLabel(d.type),
                  subtitle: '${formatPeriod(d.periodStart, d.periodEnd)} · entries close ${formatEat(d.freezeAt)}',
                  chevron: true,
                  onTap: () => context.go('/draws/${d.id}'),
                ),
              ],
            ],
          );
        },
      ),
    );
  }
}

/// The player's own draw periods: they start on the day of their first play and run back to back.
class MyPeriodsCard extends ConsumerWidget {
  const MyPeriodsCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return AppCard(
      title: 'Your draw periods',
      child: AsyncView(
        value: ref.watch(myPeriodsProvider),
        what: 'your draw periods',
        onRetry: () => ref.invalidate(myPeriodsProvider),
        data: (periods) {
          if (periods.isEmpty) {
            return Text(
              'Your weekly, monthly and quarterly periods start on the day of your first play and run 7, 30 and 90 '
              'days back to back.',
              style: t.bodyMedium,
            );
          }
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final (i, p) in periods.indexed)
                MergeSemantics(
                  child: Padding(
                    padding: EdgeInsets.only(bottom: i == periods.length - 1 ? 0 : Space.md),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Padding(
                          padding: const EdgeInsets.only(top: Space.xs),
                          child: ExcludeSemantics(child: Icon(DrawTile._icon(p.type), size: 18, color: c.primary)),
                        ),
                        const SizedBox(width: Space.md),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(drawTypeLabel(p.type), style: t.titleSmall),
                              Text(
                                '${formatPeriod(p.start, p.end)} · drawn ${formatDay(p.drawnOn)}',
                                style: t.bodySmall,
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}
