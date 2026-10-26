import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/platform/haptics.dart';
import '../../../core/platform/preferences.dart';
import '../../account/application/account_providers.dart';
import '../domain/wallet_models.dart';
import 'settlement_flow.dart';
import 'wallet_providers.dart';

/// A money request and the amount the player chose for it (read when the request is sent).
class MoneyFlow<T> {
  MoneyFlow(this._build);

  final SettlementFlow<T> Function(int Function() amount) _build;
  int amount = 0;
  late final SettlementFlow<T> flow = _build(() => amount);
}

/// One top-up from request to the server's final answer. Disposed (polling stops) when its screen closes; an
/// unsettled top-up is found again in the server's list.
final topUpFlowProvider = Provider.autoDispose<MoneyFlow<TopUp>>((ref) {
  final repo = ref.watch(walletRepositoryProvider);
  final m = MoneyFlow<TopUp>(
    (amount) =>
        SettlementFlow<TopUp>(
            send: (key) => repo.startTopUp(amountKes: amount(), idempotencyKey: key),
            fetch: repo.topUp,
            idOf: (t) => t.id,
            settled: (t) => t.status.settled,
          )
          ..onSettled = (t) {
            if (t.status == TopUpStatus.credited) ref.read(hapticsProvider).play(HapticEvent.confirmed);
            ref
              ..invalidate(walletStatementProvider)
              ..invalidate(topUpsProvider)
              ..invalidate(accountProvider); // the first credited top-up proves the phone
          },
  );
  ref.onDispose(m.flow.dispose);
  return m;
});

final withdrawFlowProvider = Provider.autoDispose<MoneyFlow<Payout>>((ref) {
  final repo = ref.watch(walletRepositoryProvider);
  final m = MoneyFlow<Payout>(
    (amount) =>
        SettlementFlow<Payout>(
            send: (key) => repo.withdraw(amountKes: amount(), idempotencyKey: key),
            fetch: repo.payout,
            idOf: (p) => p.id,
            settled: (p) => p.status.settled,
          )
          ..onSettled = (p) {
            if (p.status == PayoutStatus.paid) ref.read(hapticsProvider).play(HapticEvent.confirmed);
            ref
              ..invalidate(walletStatementProvider)
              ..invalidate(payoutsProvider);
          },
  );
  ref.onDispose(m.flow.dispose);
  return m;
});
