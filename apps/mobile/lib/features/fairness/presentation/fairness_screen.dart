import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/list_row.dart';
import '../../../core/widgets/status_pill.dart';
import '../../play/application/play_providers.dart';
import '../../play/domain/play_models.dart';
import '../application/fairness_providers.dart';
import '../domain/fair_verifier.dart';
import '../domain/fairness_models.dart';

class FairnessScreen extends ConsumerStatefulWidget {
  const FairnessScreen({super.key});

  @override
  ConsumerState<FairnessScreen> createState() => _FairnessScreenState();
}

class _FairnessScreenState extends ConsumerState<FairnessScreen> {
  var _busy = false;

  Future<void> _rotate() async {
    final ok = await confirmAction(
      context,
      title: 'Reveal the seed and start a new one?',
      facts: const [
        ('Reveals', 'The secret server seed of your current chain'),
        ('Then', 'A new chain starts, with a new committed seed'),
      ],
      note: 'After revealing, every spin made with the old seed can be checked here, on this device.',
      confirmLabel: 'Reveal and start a new chain',
    );
    if (!ok || !mounted) return;
    setState(() => _busy = true);
    final key = newIdempotencyKey();
    try {
      try {
        await ref.read(fairnessRepositoryProvider).rotate(idempotencyKey: key);
      } on ApiFailure catch (f) {
        if (!f.outcomeUncertain) rethrow;
        await ref.read(fairnessRepositoryProvider).rotate(idempotencyKey: key);
      }
      ref.invalidate(fairnessProvider);
    } on ApiFailure catch (f) {
      if (mounted) {
        final d = describeFailure(f);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              f.outcomeUncertain
                  ? 'Not confirmed: ${d.title}. Pull down to see whether the seed was revealed.'
                  : d.title,
            ),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final fairness = ref.watch(fairnessProvider);
    final plays = ref.watch(todayProvider).value?.tickets ?? const <Play>[];
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return PageScaffold(
      title: 'Fairness',
      onRefresh: () async => ref.invalidate(fairnessProvider),
      children: [
        Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ExcludeSemantics(child: Icon(Icons.verified_rounded, color: c.primary)),
            const SizedBox(width: Space.md),
            Expanded(
              child: Text(
                'Every spin is computed from a secret server seed, your client seed and a counter (the nonce). The server '
                'publishes the seed’s SHA-256 fingerprint before any spin uses it, so it cannot change the seed afterwards. '
                'Reveal the seed to check the spins yourself.',
                style: t.bodyMedium?.copyWith(color: c.inkMuted),
              ),
            ),
          ],
        ),
        AsyncView(
          value: fairness,
          what: 'your seed chains',
          onRetry: () => ref.invalidate(fairnessProvider),
          data: (f) => Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              AppCard(
                title: 'Current chain',
                trailing: const StatusPill(tone: StatusTone.pending, label: 'Committed', compact: true),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    FactRow(
                      label: 'Commitment (SHA-256 of the secret seed)',
                      value: f.active.commitment,
                      mono: true,
                      selectable: true,
                    ),
                    const SizedBox(height: Space.md),
                    FactRow(label: 'Client seed', value: f.active.clientSeed, mono: true, selectable: true),
                    const SizedBox(height: Space.md),
                    FactGrid(
                      facts: [
                        FactRow(label: 'Next nonce', value: '${f.active.nextNonce}', mono: true),
                        FactRow(label: 'Committed', value: formatEat(f.active.createdAt)),
                      ],
                    ),
                    const SizedBox(height: Space.lg),
                    OutlinedButton.icon(
                      onPressed: _busy ? null : _rotate,
                      icon: const Icon(Icons.key_rounded),
                      label: Text(_busy ? 'Revealing…' : 'Reveal seed and start a new chain'),
                    ),
                  ],
                ),
              ),
              for (final chain in f.rotated) ...[
                const SizedBox(height: Space.lg),
                _RevealedChain(chain: chain, plays: plays),
              ],
            ],
          ),
        ),
      ],
    );
  }
}

class _RevealedChain extends StatelessWidget {
  const _RevealedChain({required this.chain, required this.plays});

  final Chain chain;
  final List<Play> plays;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final mine = plays.where((p) => p.spin.chainId == chain.id).toList();
    return AppCard(
      title: 'Revealed chain',
      trailing: const StatusPill(tone: StatusTone.success, label: 'Revealed', compact: true),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          FactRow(label: 'Commitment', value: chain.commitment, mono: true, selectable: true),
          const SizedBox(height: Space.md),
          FactRow(label: 'Revealed seed', value: chain.revealedSeed ?? '—', mono: true, selectable: true),
          const SizedBox(height: Space.md),
          FactRow(label: 'Client seed', value: chain.clientSeed, mono: true, selectable: true),
          if (chain.revealedAt != null) ...[
            const SizedBox(height: Space.md),
            FactRow(label: 'Revealed', value: formatEat(chain.revealedAt!)),
          ],
          const SizedBox(height: Space.lg),
          Text("Today's spins on this chain", style: t.titleSmall),
          const SizedBox(height: Space.sm),
          if (mine.isEmpty)
            Text("None of today's plays used this chain.", style: t.bodySmall)
          else
            RowGroup(
              flat: true,
              children: [for (final p in mine) _SpinCheckRow(play: p, chain: chain)],
            ),
          if (mine.isNotEmpty) ...[
            const SizedBox(height: Space.sm),
            Text(
              'Each slot is recomputed on this device from the seed, your client seed and the nonce.',
              style: t.bodySmall?.copyWith(color: c.inkMuted),
            ),
          ],
        ],
      ),
    );
  }
}

class _SpinCheckRow extends StatelessWidget {
  const _SpinCheckRow({required this.play, required this.chain});

  final Play play;
  final Chain chain;

  @override
  Widget build(BuildContext context) {
    final s = play.spin;
    final check = verifySpin(
      revealedSeedHex: chain.revealedSeed ?? '',
      commitment: chain.commitment,
      clientSeed: s.clientSeed,
      nonce: s.nonce,
      slots: s.slots,
    );
    final (tone, label, detail) = switch (check) {
      SpinVerified(:final slot) when slot == s.slot => (
        StatusTone.success,
        'Matches',
        'Slot $slot recomputed on this device',
      ),
      SpinVerified(:final slot) => (
        StatusTone.error,
        'Mismatch',
        'Recomputed slot $slot, but the server said ${s.slot}',
      ),
      CommitmentMismatch() => (StatusTone.error, 'Mismatch', 'The revealed seed does not match the commitment'),
      InvalidSpinInput(:final reason) => (StatusTone.warning, 'Cannot check', reason),
    };
    final (color, icon) = tone.resolve(context.colors);
    return ListRow(
      icon: icon,
      iconColor: color,
      title: 'Play ${play.seq} · slot ${s.slot}',
      subtitle: detail,
      trailing: StatusPill(tone: tone, label: label, compact: true),
      semanticsLabel: 'Play ${play.seq}: $label. $detail',
    );
  }
}
