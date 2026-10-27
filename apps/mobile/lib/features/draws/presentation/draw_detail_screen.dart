import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/list_row.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/draws_providers.dart';
import '../domain/draw_models.dart';
import 'draw_copy.dart';

class DrawDetailScreen extends ConsumerWidget {
  const DrawDetailScreen({super.key, required this.id});

  final String id;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final draw = ref.watch(drawProvider(id));
    final title = draw.value == null ? 'Draw' : drawTypeLabel(draw.value!.type);
    return PageScaffold(
      title: title,
      onRefresh: () async {
        ref.invalidate(myEntriesProvider(id));
        ref.invalidate(drawProvider(id));
        await ref.read(drawProvider(id).future);
      },
      children: [
        AsyncView(
          value: draw,
          what: 'this draw',
          onRetry: () => ref.invalidate(drawProvider(id)),
          data: (d) => _DrawBody(draw: d),
        ),
      ],
    );
  }
}

class _DrawBody extends ConsumerWidget {
  const _DrawBody({required this.draw});

  final Draw draw;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final (tone, label) = drawStatus(draw.status);
    final mine = ref.watch(myEntriesProvider(draw.id));
    final myNumbers = mine.value?.entries.map((e) => e.entryNo).toSet() ?? const <int>{};

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        AppCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(formatPeriod(draw.periodStart, draw.periodEnd), style: t.headlineSmall),
              const SizedBox(height: Space.sm),
              Align(
                alignment: Alignment.centerLeft,
                child: StatusPill(tone: tone, label: label),
              ),
              const SizedBox(height: Space.lg),
              FactGrid(
                facts: [
                  FactRow(label: 'Entries close', value: formatEat(draw.freezeAt)),
                  if (draw.status != DrawStatus.open) FactRow(label: 'Entries', value: '${draw.entries}'),
                  if (draw.drandRoundAt != null)
                    FactRow(label: 'Drawn with the public beacon at', value: formatEat(draw.drandRoundAt!)),
                  if (draw.voidedReason != null) FactRow(label: 'Voided because', value: draw.voidedReason!),
                ],
              ),
            ],
          ),
        ),
        const SizedBox(height: Space.lg),
        AppCard(
          title: 'Your entries',
          child: AsyncView(
            value: mine,
            what: 'your entries',
            onRetry: () => ref.invalidate(myEntriesProvider(draw.id)),
            data: (m) {
              if (m.entries.isEmpty) {
                return Text(
                  draw.status == DrawStatus.open
                      ? 'Your entries are numbered when entries close. Plays on days in this period count.'
                      : 'You have no entries in this draw.',
                  style: t.bodyMedium?.copyWith(color: c.inkMuted),
                );
              }
              return Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('${m.entries.length} ${m.entries.length == 1 ? 'entry' : 'entries'}', style: t.titleMedium),
                  const SizedBox(height: Space.sm),
                  Wrap(
                    spacing: Space.sm,
                    runSpacing: Space.sm,
                    children: [
                      for (final e in m.entries)
                        Container(
                          padding: const EdgeInsets.symmetric(horizontal: Space.md, vertical: Space.sm),
                          decoration: BoxDecoration(
                            color: c.primaryContainer,
                            borderRadius: BorderRadius.circular(Radii.full),
                          ),
                          child: Text(
                            'No. ${e.entryNo} · ${formatDay(e.playDate)}',
                            style: t.labelMedium?.copyWith(color: c.onPrimaryContainer),
                          ),
                        ),
                    ],
                  ),
                  if (m.prizes.isNotEmpty) ...[
                    const SizedBox(height: Space.lg),
                    for (final p in m.prizes)
                      InlineBanner(
                        tone: StatusTone.success,
                        icon: Icons.emoji_events_rounded,
                        title: 'Entry ${p.entryNo} won rank ${p.rank}: ${formatKes(p.amountKes)} before tax',
                        message: 'It is paid to your M-Pesa number; see Wallet.',
                      ),
                  ],
                ],
              );
            },
          ),
        ),
        if (draw.hasResults) ...[
          const SizedBox(height: Space.lg),
          AppCard(
            title: draw.status == DrawStatus.conducted ? 'Results (awaiting approval)' : 'Results',
            padding: const EdgeInsets.only(top: Space.lg),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (draw.results.isEmpty)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(Space.lg, 0, Space.lg, Space.lg),
                    child: Text('No prizes in this draw.', style: t.bodyMedium),
                  ),
                for (final (i, r) in draw.results.indexed) ...[
                  if (i > 0) Divider(indent: Space.lg, endIndent: Space.lg, color: c.border),
                  ListRow(
                    leading: Container(
                      width: 40,
                      height: 40,
                      decoration: BoxDecoration(
                        color: r.rank == 1 ? c.gold.withValues(alpha: 0.18) : c.surfaceMuted,
                        shape: BoxShape.circle,
                      ),
                      alignment: Alignment.center,
                      child: Text('${r.rank}', style: t.titleSmall?.copyWith(color: r.rank == 1 ? c.goldText : c.ink)),
                    ),
                    title: 'Entry ${r.entryNo}${myNumbers.contains(r.entryNo) ? ' — yours' : ''}',
                    trailingText: formatKes(r.amountKes),
                  ),
                ],
              ],
            ),
          ),
        ],
        const SizedBox(height: Space.lg),
        AppCard(
          title: 'Check this draw',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                'At the close of entries the list of entries is hashed and the company commits to a secret seed. The '
                'winners come from that seed and a public drand beacon round fixed in advance, so nobody can pick them. '
                'The certificate has everything needed to recompute the winners.',
                style: t.bodySmall,
              ),
              const SizedBox(height: Space.md),
              if (draw.entryListHash == null)
                Text('These appear once entries close.', style: t.bodySmall)
              else
                AppCard(
                  flat: true,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      for (final (i, (label, value)) in [
                        if (draw.entryListHash != null) ('Entry list hash', draw.entryListHash!),
                        if (draw.seedCommit != null) ('Seed commitment', draw.seedCommit!),
                        if (draw.drandRound != null) ('drand round', '${draw.drandRound}'),
                        if (draw.revealedSeed != null) ('Revealed seed', draw.revealedSeed!),
                        if (draw.beacon != null) ('Beacon randomness', draw.beacon!.randomness),
                        if (draw.certificateHash != null) ('Certificate SHA-256', draw.certificateHash!),
                      ].indexed)
                        Padding(
                          padding: EdgeInsets.only(top: i == 0 ? 0 : Space.md),
                          child: FactRow(label: label, value: value, mono: true, selectable: true),
                        ),
                    ],
                  ),
                ),
            ],
          ),
        ),
      ],
    );
  }
}
