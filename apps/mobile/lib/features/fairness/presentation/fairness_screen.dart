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
      note: 'After revealing, every spin made with the old seed can be checked here or with tools/verifier.',
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
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(describeFailure(f).title)));
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final fairness = ref.watch(fairnessProvider);
    final plays = ref.watch(todayProvider).value?.tickets ?? const <Play>[];
    final t = Theme.of(context).textTheme;
    return PageScaffold(
      title: 'Fairness',
      onRefresh: () async => ref.invalidate(fairnessProvider),
      children: [
        Text(
          'Every spin is computed from a secret server seed, your client seed and a counter (the nonce). The server '
          'publishes the seed’s SHA-256 fingerprint before any spin uses it, so it cannot change the seed afterwards. '
          'Reveal the seed to check the spins yourself.',
          style: t.bodyMedium,
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
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    _Field('Commitment (SHA-256 of the secret seed)', f.active.commitment),
                    _Field('Client seed', f.active.clientSeed),
                    _Field('Next nonce', '${f.active.nextNonce}'),
                    _Field('Committed', formatEat(f.active.createdAt), mono: false),
                    const SizedBox(height: Space.sm),
                    OutlinedButton(
                      onPressed: _busy ? null : _rotate,
                      child: Text(_busy ? 'Revealing…' : 'Reveal seed and start a new chain'),
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
    final mine = plays.where((p) => p.spin.chainId == chain.id).toList();
    return AppCard(
      title: 'Revealed chain',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          _Field('Commitment', chain.commitment),
          _Field('Revealed seed', chain.revealedSeed ?? '—'),
          _Field('Client seed', chain.clientSeed),
          if (chain.revealedAt != null) _Field('Revealed', formatEat(chain.revealedAt!), mono: false),
          if (mine.isEmpty)
            Text("None of today's plays used this chain.", style: t.bodySmall)
          else
            for (final p in mine) _SpinCheckRow(play: p, chain: chain),
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
    final (tone, label) = switch (check) {
      SpinVerified(:final slot) when slot == s.slot => (
        StatusTone.success,
        'Play ${play.seq}: slot $slot recomputed on this device — matches',
      ),
      SpinVerified(:final slot) => (
        StatusTone.error,
        'Play ${play.seq}: recomputed slot $slot, but the server said ${s.slot}',
      ),
      CommitmentMismatch() => (StatusTone.error, 'Play ${play.seq}: the revealed seed does not match the commitment'),
      InvalidSpinInput(:final reason) => (StatusTone.warning, 'Play ${play.seq}: cannot check ($reason)'),
    };
    return Padding(
      padding: const EdgeInsets.only(top: Space.sm),
      child: StatusPill(tone: tone, label: label),
    );
  }
}

class _Field extends StatelessWidget {
  const _Field(this.label, this.value, {this.mono = true});

  final String label;
  final String value;
  final bool mono;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return Padding(
      padding: const EdgeInsets.only(bottom: Space.md),
      child: MergeSemantics(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(label, style: t.bodySmall),
            SelectableText(
              value,
              style: mono ? t.bodyMedium?.copyWith(fontFamily: 'monospace', fontSize: 13) : t.bodyLarge,
            ),
          ],
        ),
      ),
    );
  }
}
