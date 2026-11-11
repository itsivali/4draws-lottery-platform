import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../../app/theme/tokens.dart';
import '../../../../core/formatting/dates.dart';
import '../../../../core/formatting/money.dart';
import '../../../../core/widgets/app_card.dart';
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
    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(Space.xl, 0, Space.xl, Space.xl),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: s.lucky ? c.gold.withValues(alpha: 0.15) : c.primaryContainer,
                  shape: BoxShape.circle,
                ),
                child: ExcludeSemantics(
                  child: Icon(
                    s.lucky ? Icons.star_rounded : Icons.play_arrow_rounded,
                    color: s.lucky ? c.goldText : c.onPrimaryContainer,
                  ),
                ),
              ),
              const SizedBox(width: Space.md),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Semantics(header: true, child: Text('Play receipt', style: t.titleLarge)),
                    Text('${formatDay(play.playDate)} · play ${play.seq}', style: t.bodySmall),
                  ],
                ),
              ),
            ],
          ),
          const SizedBox(height: Space.xl),
          AppCard(
            flat: true,
            child: FactGrid(
              facts: [
                FactRow(
                  label: 'Paid',
                  value: '${formatKes(play.priceKes)} · ${play.status == 'refunded' ? 'refunded' : 'paid'}',
                ),
                FactRow(label: 'When', value: formatEat(play.createdAt)),
                FactRow(label: 'Spin', value: 'Slot ${s.slot} of ${s.slots}${s.lucky ? ' — a lucky slot' : ''}'),
                FactRow(label: 'Lucky slots', value: 'Below ${s.luckySlots}'),
              ],
            ),
          ),
          const SizedBox(height: Space.xl),
          Text('Provably fair record', style: t.titleMedium),
          const SizedBox(height: Space.sm),
          Text(
            'The server committed to a secret seed before this spin by publishing its fingerprint (the commitment). '
            'When you reveal the seed in Fairness, anyone can recompute this slot from the seed, your client seed and '
            'the nonce — on this device too.',
            style: t.bodySmall,
          ),
          const SizedBox(height: Space.md),
          AppCard(
            flat: true,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                FactRow(label: 'Ticket', value: play.ticketId, mono: true, selectable: true),
                const SizedBox(height: Space.md),
                FactRow(
                  label: 'Commitment (SHA-256 of the server seed)',
                  value: s.commitment,
                  mono: true,
                  selectable: true,
                ),
                const SizedBox(height: Space.md),
                FactRow(label: 'Client seed', value: s.clientSeed, mono: true, selectable: true),
                const SizedBox(height: Space.md),
                FactGrid(
                  facts: [
                    FactRow(label: 'Nonce', value: '${s.nonce}', mono: true, selectable: true),
                    FactRow(label: 'Seed chain', value: s.chainId, mono: true, selectable: true),
                  ],
                ),
              ],
            ),
          ),
          const SizedBox(height: Space.lg),
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
