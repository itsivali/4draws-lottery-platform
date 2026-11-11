import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/dates.dart';
import '../../../core/formatting/money.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/coin.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/glow.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/list_row.dart';
import '../../../core/widgets/skeleton.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/wallet_providers.dart';
import '../domain/wallet_models.dart';
import 'receipt_claim_card.dart';
import 'wallet_copy.dart';

const _preview = 5;

class WalletScreen extends ConsumerStatefulWidget {
  const WalletScreen({super.key});

  @override
  ConsumerState<WalletScreen> createState() => _WalletScreenState();
}

class _WalletScreenState extends ConsumerState<WalletScreen> {
  var _allTopUps = false;
  var _allPayouts = false;

  @override
  Widget build(BuildContext context) {
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
        // the balance as a big light numeral over soft glows (redesign 2026-11-11)
        GlowPanel(
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
                  Text('Confirmed balance', style: t.labelLarge?.copyWith(color: c.inkMuted)),
                  const SizedBox(height: Space.xs),
                  Row(
                    children: [
                      Expanded(
                        child: AnimatedSwitcher(
                          duration: context.reduceMotion ? Duration.zero : Motion.standard,
                          layoutBuilder: (current, previous) =>
                              Stack(alignment: AlignmentDirectional.centerStart, children: [...previous, ?current]),
                          child: BigAmount(key: ValueKey(w.balanceKes), text: formatKes(w.balanceKes)),
                        ),
                      ),
                      const SizedBox(width: Space.sm),
                      const Coin3D(size: 72),
                    ],
                  ),
                  const SizedBox(height: Space.sm),
                  Text('Only money the server has confirmed is shown here.', style: t.bodySmall),
                ],
              ),
            ),
          ),
        ),
        if (wallet.value case final w? when w.debtKes > 0)
          InlineBanner(
            tone: StatusTone.warning,
            title: 'Owed after a reversed top-up: ${formatKes(w.debtKes)}',
            message: 'Play and withdrawals are paused until it is repaid; your next top-up repays it first.',
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
        _Section(
          title: 'Top-ups',
          value: topUps,
          what: 'your top-ups',
          onRetry: () => ref.invalidate(topUpsProvider),
          emptyIcon: Icons.add_card_rounded,
          emptyText: 'No top-ups yet.',
          showAll: _allTopUps,
          onShowAll: () => setState(() => _allTopUps = true),
          row: (TopUp tp) {
            final (tone, label) = topUpStatus(tp.status);
            final (color, icon) = tone.resolve(c);
            final claimLine = receiptClaimLine(tp);
            final canAct = tp.status == TopUpStatus.unconfirmed || tp.status == TopUpStatus.failed;
            return ListRow(
              icon: icon,
              iconColor: color,
              title: formatKes(tp.amountConfirmedKes ?? tp.amountRequestedKes),
              subtitle: claimLine == null ? formatEat(tp.createdAt) : '${formatEat(tp.createdAt)}\n$claimLine',
              trailing: StatusPill(tone: tone, label: label, compact: true),
              onTap: canAct ? () => _showReceiptSheet(context, tp) : null,
            );
          },
        ),
        _Section(
          title: 'Withdrawals and prizes',
          value: payouts,
          what: 'your payouts',
          onRetry: () => ref.invalidate(payoutsProvider),
          emptyIcon: Icons.emoji_events_outlined,
          emptyText: 'No withdrawals or prizes yet.',
          showAll: _allPayouts,
          onShowAll: () => setState(() => _allPayouts = true),
          row: (Payout p) {
            final (tone, label) = payoutStatus(p.status);
            final (color, icon) = tone.resolve(c);
            final details = [
              if (p.taxKes > 0) 'Tax withheld ${formatKes(p.taxKes)}',
              if (p.debtRepaidKes > 0) 'Repaid debt ${formatKes(p.debtRepaidKes)}',
              if (p.receipt != null) 'M-Pesa receipt ${p.receipt}',
              formatEat(p.createdAt),
            ];
            return ListRow(
              icon: p.kind == 'prize' ? Icons.emoji_events_rounded : icon,
              iconColor: p.kind == 'prize' ? c.goldText : color,
              title: '${p.kind == 'prize' ? 'Prize' : 'Withdrawal'} · ${formatKes(p.netKes)} to M-Pesa',
              subtitle: details.join(' · '),
              trailing: StatusPill(tone: tone, label: label, compact: true),
            );
          },
        ),
        AppCard(
          title: 'Activity',
          icon: Icons.receipt_long_rounded,
          padding: EdgeInsets.zero,
          child: AsyncView(
            value: wallet,
            what: 'your activity',
            onRetry: () => ref.invalidate(walletStatementProvider),
            data: (w) => w.transactions.isEmpty
                ? const EmptyState(icon: Icons.receipt_long_rounded, title: 'No activity yet.')
                : Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      for (final (i, x) in w.transactions.indexed) ...[
                        if (i > 0) Divider(indent: Space.lg, endIndent: Space.lg, color: c.border),
                        ListRow(
                          icon: _txIcon(x.kind),
                          iconColor: x.amountKes > 0 ? c.success : c.inkMuted,
                          title: txLabel(x.kind),
                          subtitle: formatEat(x.createdAt),
                          trailingText: formatKes(x.amountKes, signed: true),
                          trailingColor: x.amountKes > 0 ? c.success : c.ink,
                        ),
                      ],
                    ],
                  ),
          ),
        ),
      ],
    );
  }

  static IconData _txIcon(String kind) => switch (kind) {
    'deposit' => Icons.add_rounded,
    'stake' => Icons.donut_large_rounded,
    'withdrawal' => Icons.arrow_outward_rounded,
    'prize' => Icons.emoji_events_rounded,
    'refund' => Icons.undo_rounded,
    'reversal' => Icons.replay_rounded,
    'debt_recovery' => Icons.handshake_rounded,
    _ => Icons.swap_horiz_rounded,
  };
}

/// A titled list with the first few rows and a "Show all" when there are more.
class _Section<T> extends StatelessWidget {
  const _Section({
    required this.title,
    required this.value,
    required this.what,
    required this.onRetry,
    required this.emptyIcon,
    required this.emptyText,
    required this.showAll,
    required this.onShowAll,
    required this.row,
  });

  final String title;
  final AsyncValue<List<T>> value;
  final String what;
  final VoidCallback onRetry;
  final IconData emptyIcon;
  final String emptyText;
  final bool showAll;
  final VoidCallback onShowAll;
  final Widget Function(T) row;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    return AppCard(
      title: title,
      padding: const EdgeInsets.only(top: Space.lg),
      child: AsyncView(
        value: value,
        what: what,
        onRetry: onRetry,
        data: (list) {
          if (list.isEmpty) return EmptyState(icon: emptyIcon, title: emptyText);
          final shown = showAll ? list : list.take(_preview).toList();
          return Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              for (final (i, item) in shown.indexed) ...[
                if (i > 0) Divider(indent: Space.lg, endIndent: Space.lg, color: c.border),
                row(item),
              ],
              if (list.length > shown.length)
                Padding(
                  padding: const EdgeInsets.fromLTRB(Space.sm, Space.xs, Space.sm, Space.sm),
                  child: TextButton(onPressed: onShowAll, child: Text('Show all ${list.length}')),
                ),
            ],
          );
        },
      ),
    );
  }
}

/// The receipt from the player's M-Pesa message, for a top-up in the history that was not credited.
Future<void> _showReceiptSheet(BuildContext context, TopUp topUp) => showModalBottomSheet<void>(
  context: context,
  isScrollControlled: true,
  showDragHandle: true,
  builder: (sheet) => Padding(
    padding: EdgeInsets.fromLTRB(Space.lg, 0, Space.lg, Space.lg + MediaQuery.viewInsetsOf(sheet).bottom),
    child: Consumer(
      builder: (context, ref, _) {
        // the row's top-up is re-read after the receipt is taken, so the sheet follows the server's view of it
        final latest = ref.watch(topUpsProvider).value?.where((x) => x.id == topUp.id).firstOrNull ?? topUp;
        return SingleChildScrollView(
          child: ReceiptClaimCard(topUp: latest, onChanged: () => ref.invalidate(topUpsProvider)),
        );
      },
    ),
  ),
);
