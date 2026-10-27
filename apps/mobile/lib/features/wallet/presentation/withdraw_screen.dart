import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/formatting/money.dart';
import '../../../core/formatting/phone.dart';
import '../../../core/widgets/app_card.dart';
import '../../../core/widgets/async_view.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/confirm_sheet.dart';
import '../../../core/widgets/content_frame.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../../account/application/account_providers.dart';
import '../../account/domain/account_models.dart';
import '../application/money_flows.dart';
import '../application/settlement_flow.dart';
import '../application/wallet_providers.dart';
import '../domain/wallet_models.dart';
import 'amount_form.dart';
import 'money_flow_view.dart';

class WithdrawScreen extends ConsumerStatefulWidget {
  const WithdrawScreen({super.key});

  @override
  ConsumerState<WithdrawScreen> createState() => _WithdrawScreenState();
}

class _WithdrawScreenState extends ConsumerState<WithdrawScreen> {
  final _form = GlobalKey<FormState>();
  final _amount = TextEditingController();

  @override
  void dispose() {
    _amount.dispose();
    super.dispose();
  }

  Future<void> _submit(Account account, WalletStatement wallet, MoneyFlow<Payout> m) async {
    if (!_form.currentState!.validate()) return;
    final amount = int.parse(_amount.text);
    final ok = await confirmAction(
      context,
      title: 'Withdraw ${formatKes(amount)}?',
      facts: [
        ('Amount', formatKes(amount)),
        ('Sent by M-Pesa to', formatMsisdn(account.msisdn)),
        ('Balance after', formatKes(wallet.balanceKes - amount)),
      ],
      note:
          'The amount leaves your wallet now. If M-Pesa cannot pay it, it comes back to your wallet. Large amounts '
          'may wait for a manual check.',
      confirmLabel: 'Withdraw ${formatKes(amount)}',
    );
    if (!ok || !mounted) return;
    m.amount = amount;
    m.flow.submit();
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider);
    final wallet = ref.watch(walletStatementProvider);
    final m = ref.watch(withdrawFlowProvider);
    final t = Theme.of(context).textTheme;
    final c = context.colors;
    return PageScaffold(
      title: 'Withdraw to M-Pesa',
      children: [
        AsyncView(
          value: account,
          what: 'your account',
          onRetry: () => ref.invalidate(accountProvider),
          data: (a) => AsyncView(
            value: wallet,
            what: 'your wallet',
            onRetry: () => ref.invalidate(walletStatementProvider),
            data: (w) {
              final (blocked, action, route) = a.kycStatus != KycStatus.verified
                  ? (
                      'Withdrawals need a verified national ID.',
                      a.kycStatus == KycStatus.none ? 'Verify my ID' : null,
                      '/account/verify',
                    )
                  : !a.phoneVerified
                  ? ('Withdrawals go to a proved number. Your first top-up proves it.', 'Top up', '/wallet/top-up')
                  : w.debtKes > 0
                  ? (
                      'Withdrawals are paused while you owe money from a reversed top-up. Your next top-up repays it.',
                      'Top up',
                      '/wallet/top-up',
                    )
                  : (null, null, null);
              if (blocked != null) {
                return InlineBanner(
                  tone: StatusTone.warning,
                  title: "You can't withdraw yet",
                  message: blocked,
                  actionLabel: action,
                  onAction: action == null ? null : () => context.go(route!),
                );
              }
              return ValueListenableBuilder<SettlementState<Payout>>(
                valueListenable: m.flow.state,
                builder: (context, s, _) {
                  final started = s.stage != SettlementStage.ready && s.stage != SettlementStage.refused;
                  return Form(
                    key: _form,
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        AppCard(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.stretch,
                            children: [
                              Row(
                                children: [
                                  Expanded(
                                    child: Column(
                                      crossAxisAlignment: CrossAxisAlignment.start,
                                      children: [
                                        Text('Available', style: t.labelSmall),
                                        Text(formatKes(w.balanceKes), style: t.headlineSmall),
                                      ],
                                    ),
                                  ),
                                  Expanded(
                                    child: Column(
                                      crossAxisAlignment: CrossAxisAlignment.start,
                                      children: [
                                        Text('Paid to', style: t.labelSmall),
                                        Text(formatMsisdn(a.msisdn), style: t.titleMedium),
                                      ],
                                    ),
                                  ),
                                ],
                              ),
                              Divider(height: Space.xxl, color: c.border),
                              AmountField(
                                controller: _amount,
                                min: 50,
                                max: 150000,
                                quick: const [100, 500, 1000],
                                label: 'Amount',
                                enabled: !started,
                                extraValidator: (n) => n > w.balanceKes ? 'That is more than your balance.' : null,
                              ),
                              const SizedBox(height: Space.xl),
                              if (!started)
                                BusyButton(label: 'Continue', busy: false, onPressed: () => _submit(a, w, m)),
                            ],
                          ),
                        ),
                        if (s.stage != SettlementStage.ready) ...[
                          const SizedBox(height: Space.lg),
                          AppCard(
                            child: SettlementStatusView<Payout>(
                              flow: m.flow,
                              pendingText: 'Sending to your M-Pesa. This is final only once M-Pesa confirms it.',
                              describe: (p) => switch (p.status) {
                                PayoutStatus.paid => (
                                  StatusTone.success,
                                  'Paid',
                                  '${formatKes(p.netKes)} sent. M-Pesa receipt ${p.receipt ?? '—'}.',
                                ),
                                PayoutStatus.failed => (
                                  StatusTone.error,
                                  'Not paid',
                                  'M-Pesa could not pay it. The amount is back in your wallet.',
                                ),
                                PayoutStatus.processing => (StatusTone.pending, 'Processing', ''),
                              },
                              onDone: () => context.go('/wallet'),
                            ),
                          ),
                        ],
                      ],
                    ),
                  );
                },
              );
            },
          ),
        ),
      ],
    );
  }
}
