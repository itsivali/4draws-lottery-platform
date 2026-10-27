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
  int _amount = 0;
  late final SettlementFlow<T> flow = _build(() => _amount);

  /// Sends one request for [amountKes]. The amount is fixed for that request: a retry after a lost answer sends the
  /// same one, whatever the form shows by then.
  void submit(int amountKes) {
    if (flow.state.value.busy) return;
    _amount = amountKes;
    flow.submit();
  }
}

/// One top-up from request to the server's final answer. The flow lives for the session, not the screen: leaving the
/// screen mid-request keeps the idempotency key, so coming back never starts a second prompt for the same top-up.
/// It is rebuilt on every session change (app.dart).
final topUpFlowProvider = Provider<MoneyFlow<TopUp>>((ref) {
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

final withdrawFlowProvider = Provider<MoneyFlow<Payout>>((ref) {
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
