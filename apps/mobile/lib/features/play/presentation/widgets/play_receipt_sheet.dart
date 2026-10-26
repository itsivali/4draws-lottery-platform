import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../../app/theme/tokens.dart';
import '../../../../core/formatting/dates.dart';
import '../../../../core/formatting/money.dart';
import '../../domain/play_models.dart';

/// A play's receipt: what was paid and the spin's provably fair record.
Future<void> showPlayReceipt(BuildContext context, Play play) => showModalBottomSheet<void>(
  context: context,
  isScrollControlled: true,
  useSafeArea: true,
  builder: (context) => _Receipt(play: play),
);

class _Receipt extends StatelessWidget {
  const _Receipt({required this.play});

  final Play play;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    final s = play.spin;
    Widget row(String label, String value, {bool mono = false}) => Padding(
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
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(Space.xl, 0, Space.xl, Space.xl),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Semantics(header: true, child: Text('Play receipt', style: t.titleLarge)),
          const SizedBox(height: Space.lg),
          row('Ticket', play.ticketId, mono: true),
          row('Play day', '${formatDay(play.playDate)} · play ${play.seq} of 4'),
          row(
            'Paid',
            '${formatKes(play.priceKes)} · ${play.status == 'refunded' ? 'refunded' : 'paid'} · ${formatEat(play.createdAt)}',
          ),
          row(
            'Spin',
            'Slot ${s.slot} of ${s.slots}${s.lucky ? ' — a lucky slot' : ''} (slots below ${s.luckySlots} are lucky)',
          ),
          const Divider(),
          const SizedBox(height: Space.md),
          Text('Provably fair record', style: t.titleMedium),
          const SizedBox(height: Space.sm),
          Text(
            'The server committed to a secret seed before this spin by publishing its fingerprint (the commitment). '
            'When you reveal the seed in Fairness, anyone can recompute this slot from the seed, your client seed and '
            'the nonce — on this device too.',
            style: t.bodySmall,
          ),
          const SizedBox(height: Space.md),
          row('Commitment (SHA-256 of the server seed)', s.commitment, mono: true),
          row('Client seed', s.clientSeed, mono: true),
          row('Nonce', '${s.nonce}', mono: true),
          row('Seed chain', s.chainId, mono: true),
          const SizedBox(height: Space.sm),
          OutlinedButton.icon(
            icon: Icon(Icons.verified_rounded, color: c.primary),
            label: const Text('Open Fairness to verify'),
            onPressed: () {
              Navigator.of(context).pop();
              context.go('/account/fairness');
            },
          ),
        ],
      ),
    );
  }
}
