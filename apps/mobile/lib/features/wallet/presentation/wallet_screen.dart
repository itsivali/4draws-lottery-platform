import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/skeleton.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/wallet_providers.dart';
import 'wallet_copy.dart';

class WalletScreen extends ConsumerWidget {
  const WalletScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final wallet = ref.watch(walletStatementProvider);
    final topUps = ref.watch(topUpsProvider);
    final payouts = ref.watch(payoutsProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return PageScaffold(
      title: 'Wallet',
      onRefresh: () async {
        ref
          ..invalidate(topUpsProvider)
          ..invalidate(payoutsProvider);
        ref.invalidate(walletStatementProvider);
        await ref.read(walletStatementProvider.future);
      },
      children: [
        AppCard(
          child: AsyncView(
            value: wallet,
            what: 'your wallet',
            onRetry: () => ref.invalidate(walletStatementProvider),
            loading: const Skeleton(height: 64, width: 220),
            data: (w) => Semantics(
              container: true,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Confirmed balance', style: t.bodySmall),
                  AnimatedSwitcher(
                    duration: context.reduceMotion ? Duration.zero : Motion.standard,
                    child: Text(formatKes(w.balanceKes), key: ValueKey(w.balanceKes), style: t.displayMedium),
                  ),
                  Text('Only money the server has confirmed is shown here.', style: t.bodySmall),
                  if (w.debtKes > 0) ...[
                    const SizedBox(height: Space.md),
                    StatusPill(
                      tone: StatusTone.warning,
                      label: 'Owed after a reversed top-up: ${formatKes(w.debtKes)}',
                    ),
                    const SizedBox(height: Space.xs),
                    Text(
                      'Play and withdrawals are paused until it is repaid; your next top-up repays it first.',
                      style: t.bodySmall,
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
        Row(
          children: [
            Expanded(
              child: FilledButton.icon(
                onPressed: () => context.go('/wallet/top-up'),
                icon: const Icon(Icons.add_rounded),
                label: const Text('Top up'),
              ),
            ),
            const SizedBox(width: Space.md),
            Expanded(
              child: OutlinedButton.icon(
                onPressed: () => context.go('/wallet/withdraw'),
                icon: const Icon(Icons.arrow_outward_rounded),
                label: const Text('Withdraw'),
              ),
            ),
          ],
        ),
        AppCard(
          title: 'Top-ups',
          child: AsyncView(
            value: topUps,
            what: 'your top-ups',
            onRetry: () => ref.invalidate(topUpsProvider),
            data: (list) => list.isEmpty
                ? Text('No top-ups yet.', style: t.bodyMedium)
                : Column(
                    children: [
                      for (final tp in list.take(10))
                        ListTile(
                          contentPadding: EdgeInsets.zero,
                          title: Text(formatKes(tp.amountConfirmedKes ?? tp.amountRequestedKes)),
                          subtitle: Padding(
                            padding: const EdgeInsets.only(top: Space.xs),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                StatusPill(tone: topUpStatus(tp.status).$1, label: topUpStatus(tp.status).$2),
                                Text(formatEat(tp.createdAt), style: t.bodySmall),
                              ],
                            ),
                          ),
                        ),
                    ],
                  ),
          ),
        ),
        AppCard(
          title: 'Withdrawals and prizes',
          child: AsyncView(
            value: payouts,
            what: 'your payouts',
            onRetry: () => ref.invalidate(payoutsProvider),
            data: (list) => list.isEmpty
                ? Text('No withdrawals or prizes yet.', style: t.bodyMedium)
                : Column(
                    children: [
                      for (final p in list.take(10))
                        ListTile(
                          contentPadding: EdgeInsets.zero,
                          title: Text(
                            '${p.kind == 'prize' ? 'Prize' : 'Withdrawal'} · ${formatKes(p.netKes)} to M-Pesa',
                          ),
                          subtitle: Padding(
                            padding: const EdgeInsets.only(top: Space.xs),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                StatusPill(tone: payoutStatus(p.status).$1, label: payoutStatus(p.status).$2),
                                if (p.taxKes > 0) Text('Tax withheld ${formatKes(p.taxKes)}', style: t.bodySmall),
                                if (p.debtRepaidKes > 0)
                                  Text('Repaid debt ${formatKes(p.debtRepaidKes)}', style: t.bodySmall),
                                if (p.receipt != null) Text('M-Pesa receipt ${p.receipt}', style: t.bodySmall),
                                Text(formatEat(p.createdAt), style: t.bodySmall),
                              ],
                            ),
                          ),
                        ),
                    ],
                  ),
          ),
        ),
        AppCard(
          title: 'Activity',
          child: AsyncView(
            value: wallet,
            what: 'your activity',
            onRetry: () => ref.invalidate(walletStatementProvider),
            data: (w) => w.transactions.isEmpty
                ? Text('No activity yet.', style: t.bodyMedium)
                : Column(
                    children: [
                      for (final x in w.transactions)
                        ListTile(
                          contentPadding: EdgeInsets.zero,
                          title: Text(txLabel(x.kind)),
                          subtitle: Text(formatEat(x.createdAt)),
                          trailing: Text(
                            formatKes(x.amountKes, signed: true),
                            style: t.titleMedium?.copyWith(color: x.amountKes > 0 ? c.success : c.ink),
                          ),
                        ),
                    ],
                  ),
          ),
        ),
      ],
    );
  }
}
