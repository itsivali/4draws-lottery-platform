import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/content_frame.dart';
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

    Widget fact(String l, String v, {bool mono = false}) => Padding(
      padding: const EdgeInsets.only(bottom: Space.md),
      child: MergeSemantics(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(l, style: t.bodySmall),
            SelectableText(
              v,
              style: mono ? t.bodyMedium?.copyWith(fontFamily: 'monospace', fontSize: 13) : t.bodyLarge,
            ),
          ],
        ),
      ),
    );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        AppCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(formatPeriod(draw.periodStart, draw.periodEnd), style: t.titleLarge),
              const SizedBox(height: Space.sm),
              StatusPill(tone: tone, label: label),
              const SizedBox(height: Space.md),
              fact('Entries close', formatEat(draw.freezeAt)),
              if (draw.status != DrawStatus.open) fact('Entries', '${draw.entries}'),
              if (draw.drandRoundAt != null) fact('Drawn with the public beacon at', formatEat(draw.drandRoundAt!)),
              if (draw.voidedReason != null) fact('Voided because', draw.voidedReason!),
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
                  style: t.bodyMedium,
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
                      for (final e in m.entries) Chip(label: Text('No. ${e.entryNo} · ${formatDay(e.playDate)}')),
                    ],
                  ),
                  if (m.prizes.isNotEmpty) ...[
                    const SizedBox(height: Space.md),
                    for (final p in m.prizes)
                      Row(
                        children: [
                          ExcludeSemantics(child: Icon(Icons.emoji_events_rounded, color: c.goldText)),
                          const SizedBox(width: Space.sm),
                          Expanded(
                            child: Text(
                              'Entry ${p.entryNo} won rank ${p.rank}: ${formatKes(p.amountKes)} before tax. '
                              'It is paid to your M-Pesa number; see Wallet.',
                              style: t.bodyMedium,
                            ),
                          ),
                        ],
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
            child: Column(
              children: [
                if (draw.results.isEmpty) Text('No prizes in this draw.', style: t.bodyMedium),
                for (final r in draw.results)
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: CircleAvatar(
                      backgroundColor: c.surfaceMuted,
                      foregroundColor: c.ink,
                      child: Text('${r.rank}'),
                    ),
                    title: Text('Entry ${r.entryNo}${myNumbers.contains(r.entryNo) ? ' — yours' : ''}'),
                    trailing: Text(formatKes(r.amountKes), style: t.titleMedium),
                  ),
              ],
            ),
          ),
        ],
        const SizedBox(height: Space.lg),
        AppCard(
          title: 'Check this draw',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'At the close of entries the list of entries is hashed and the company commits to a secret seed. The '
                'winners come from that seed and a public drand beacon round fixed in advance, so nobody can pick them. '
                'The certificate has everything needed to recompute the winners (tools/verifier draw).',
                style: t.bodySmall,
              ),
              const SizedBox(height: Space.md),
              if (draw.entryListHash != null) fact('Entry list hash', draw.entryListHash!, mono: true),
              if (draw.seedCommit != null) fact('Seed commitment', draw.seedCommit!, mono: true),
              if (draw.drandRound != null) fact('drand round', '${draw.drandRound}', mono: true),
              if (draw.revealedSeed != null) fact('Revealed seed', draw.revealedSeed!, mono: true),
              if (draw.beacon != null) fact('Beacon randomness', draw.beacon!.randomness, mono: true),
              if (draw.certificateHash != null) fact('Certificate SHA-256', draw.certificateHash!, mono: true),
              if (draw.entryListHash == null) Text('These appear once entries close.', style: t.bodySmall),
            ],
          ),
        ),
      ],
    );
  }
}
