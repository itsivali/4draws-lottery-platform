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
import '../../../core/widgets/status_pill.dart';
import '../../account/application/account_providers.dart';
import '../../account/domain/account_models.dart';
import '../application/money_flows.dart';
import '../application/settlement_flow.dart';
import '../application/wallet_providers.dart';
import '../domain/wallet_models.dart';
import 'amount_form.dart';
import 'money_flow_view.dart';

class TopUpScreen extends ConsumerStatefulWidget {
  const TopUpScreen({super.key});

  @override
  ConsumerState<TopUpScreen> createState() => _TopUpScreenState();
}

class _TopUpScreenState extends ConsumerState<TopUpScreen> {
  final _form = GlobalKey<FormState>();
  final _amount = TextEditingController();

  @override
  void dispose() {
    _amount.dispose();
    super.dispose();
  }

  Future<void> _submit(Account account, MoneyFlow<TopUp> m) async {
    if (!_form.currentState!.validate()) return;
    final amount = int.parse(_amount.text);
    final ok = await confirmAction(
      context,
      title: 'Top up ${formatKes(amount)}?',
      facts: [('Amount', formatKes(amount)), ('M-Pesa prompt goes to', formatMsisdn(account.msisdn))],
      note:
          'Approve the prompt on your phone with your M-Pesa PIN. Your wallet is credited only after M-Pesa '
          "confirms the payment, less the government's excise duty on deposits.",
      confirmLabel: 'Send M-Pesa prompt',
    );
    if (!ok || !mounted) return;
    m.amount = amount;
    m.flow.submit();
  }

  @override
  Widget build(BuildContext context) {
    final account = ref.watch(accountProvider);
    final topUps = ref.watch(topUpsProvider);
    final m = ref.watch(topUpFlowProvider);
    final t = Theme.of(context).textTheme;
    return PageScaffold(
      title: 'Top up with M-Pesa',
      children: [
        AsyncView(
          value: account,
          what: 'your account',
          onRetry: () => ref.invalidate(accountProvider),
          data: (a) {
            if (a.kycStatus != KycStatus.verified) {
              return AppCard(
                title: 'Verify your identity first',
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Top-ups need a verified national ID.', style: t.bodyMedium),
                    const SizedBox(height: Space.md),
                    if (a.kycStatus == KycStatus.none)
                      FilledButton(onPressed: () => context.go('/account/verify'), child: const Text('Verify my ID')),
                  ],
                ),
              );
            }
            return ValueListenableBuilder<SettlementState<TopUp>>(
              valueListenable: m.flow.state,
              builder: (context, s, _) {
                final pendingOnServer = topUps.value?.where((x) => x.status == TopUpStatus.pending).firstOrNull;
                if (s.stage == SettlementStage.ready && pendingOnServer != null) {
                  return _PendingElsewhere(topUp: pendingOnServer, onTrack: () => m.flow.track(pendingOnServer));
                }
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
                            Text(
                              'The prompt goes to your account number, ${formatMsisdn(a.msisdn)}. It cannot be sent to another number.',
                              style: t.bodyMedium,
                            ),
                            if (!a.phoneVerified) ...[
                              const SizedBox(height: Space.sm),
                              Text(
                                'Approving this first prompt also confirms that the number is yours.',
                                style: t.bodySmall,
                              ),
                            ],
                            const SizedBox(height: Space.lg),
                            AmountField(
                              controller: _amount,
                              min: 10,
                              max: 250000,
                              quick: const [100, 200, 500, 1000],
                              label: 'Amount',
                              enabled: !started,
                            ),
                            const SizedBox(height: Space.lg),
                            if (!started) BusyButton(label: 'Continue', busy: false, onPressed: () => _submit(a, m)),
                          ],
                        ),
                      ),
                      if (s.stage != SettlementStage.ready) ...[
                        const SizedBox(height: Space.lg),
                        AppCard(
                          child: SettlementStatusView<TopUp>(
                            flow: m.flow,
                            pendingText: 'Approve the M-Pesa prompt on your phone.',
                            describe: (x) => switch (x.status) {
                              TopUpStatus.credited => (
                                StatusTone.success,
                                'Credited',
                                '${formatKes(x.amountConfirmedKes ?? x.amountRequestedKes)} paid. Your balance is updated.',
                              ),
                              TopUpStatus.failed => (
                                StatusTone.error,
                                'Not completed',
                                'M-Pesa did not confirm a payment, so nothing was taken. You can try again.',
                              ),
                              TopUpStatus.reversed => (
                                StatusTone.warning,
                                'Reversed',
                                'M-Pesa took this payment back.',
                              ),
                              TopUpStatus.pending => (StatusTone.pending, 'Pending', ''),
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
      ],
    );
  }
}

class _PendingElsewhere extends StatelessWidget {
  const _PendingElsewhere({required this.topUp, required this.onTrack});

  final TopUp topUp;
  final VoidCallback onTrack;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return AppCard(
      title: 'A top-up is still pending',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const StatusPill(tone: StatusTone.pending, label: 'Pending M-Pesa confirmation'),
          const SizedBox(height: Space.sm),
          Text(
            'Your top-up of ${formatKes(topUp.amountRequestedKes)} has not been confirmed yet. To avoid paying twice, a new '
            'top-up can start once this one is settled.',
            style: t.bodyMedium,
          ),
          const SizedBox(height: Space.md),
          FilledButton(onPressed: onTrack, child: const Text('Follow its status')),
        ],
      ),
    );
  }
}
