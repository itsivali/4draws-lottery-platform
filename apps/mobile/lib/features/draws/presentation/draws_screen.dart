import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/content_frame.dart';
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
          style: Theme.of(context).textTheme.bodyMedium,
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
            for (final t in DrawType.values)
              ChoiceChip(
                label: Text(drawTypeLabel(t).replaceFirst(' draw', '')),
                selected: _type == t,
                onSelected: (_) => setState(() => _type = t),
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
              return Text('No draws to show yet.', style: Theme.of(context).textTheme.bodyMedium);
            }
            return Column(
              children: [
                for (final d in shown) ...[DrawTile(draw: d), const SizedBox(height: Space.md)],
              ],
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
    final (tone, label) = drawStatus(draw.status);
    return Card(
      clipBehavior: Clip.antiAlias,
      child: InkWell(
        onTap: () => context.go('/draws/${draw.id}'),
        child: Padding(
          padding: const EdgeInsets.all(Space.lg),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(drawTypeLabel(draw.type), style: t.titleMedium),
                    Text(formatPeriod(draw.periodStart, draw.periodEnd), style: t.bodySmall),
                    const SizedBox(height: Space.sm),
                    StatusPill(tone: tone, label: label),
                    if (draw.status != DrawStatus.open) ...[
                      const SizedBox(height: Space.xs),
                      Text('${draw.entries} entries', style: t.bodySmall),
                    ],
                  ],
                ),
              ),
              const ExcludeSemantics(child: Icon(Icons.chevron_right_rounded)),
            ],
          ),
        ),
      ),
    );
  }
}

/// The open draws, for Home.
class OpenDrawsCard extends ConsumerWidget {
  const OpenDrawsCard({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final draws = ref.watch(drawsProvider);
    return AppCard(
      title: 'Open draws',
      trailing: TextButton(onPressed: () => context.go('/draws'), child: const Text('All draws')),
      child: AsyncView(
        value: draws,
        what: 'the draws',
        onRetry: () => ref.invalidate(drawsProvider),
        data: (list) {
          final open = list.where((d) => d.status == DrawStatus.open).take(4).toList();
          if (open.isEmpty) {
            return Text('No draw is taking entries right now.', style: Theme.of(context).textTheme.bodyMedium);
          }
          return Column(
            children: [
              for (final d in open)
                ListTile(
                  contentPadding: EdgeInsets.zero,
                  title: Text(drawTypeLabel(d.type)),
                  subtitle: Text(
                    '${formatPeriod(d.periodStart, d.periodEnd)} · entries close ${formatEat(d.freezeAt)}',
                  ),
                  trailing: const Icon(Icons.chevron_right_rounded),
                  onTap: () => context.go('/draws/${d.id}'),
                ),
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
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              for (final p in periods) ...[
                Text(drawTypeLabel(p.type), style: t.titleSmall),
                Text('${formatPeriod(p.start, p.end)} · drawn ${formatDay(p.drawnOn)}', style: t.bodySmall),
                const SizedBox(height: Space.sm),
              ],
            ],
          );
        },
      ),
    );
  }
}
