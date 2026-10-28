import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../wallet/application/money_flows.dart';
import '../../wallet/application/settlement_flow.dart';
import '../../wallet/domain/wallet_models.dart';
import 'play_controller.dart';

/// Where a "pay with M-Pesa and spin" stands.
enum PayAndPlayStage {
  idle,

  /// The top-up for the price is on its way: an M-Pesa prompt on the player's phone, then the server's confirmation.
  paying,

  /// M-Pesa did not pay it (refused, failed, or still unconfirmed after the wait): nothing was played.
  notPaid,
}

/// "Pay with M-Pesa and spin" (owner, 2026-10-28): when the wallet cannot cover a play, the price is charged to the
/// player's own M-Pesa number as a top-up of exactly that amount, and the play is made the moment the server credits
/// it. It reuses the session's top-up flow, so a prompt already in progress is never doubled, and the play itself goes
/// through the same controller as a play from the wallet (one key, recovered if the answer is lost).
class PayAndPlay extends Notifier<PayAndPlayStage> {
  void Function()? _unlisten;

  @override
  PayAndPlayStage build() {
    ref.onDispose(() => _unlisten?.call());
    return PayAndPlayStage.idle;
  }

  /// Charges [priceKes] to the player's M-Pesa and spins once it is credited.
  void start(int priceKes) {
    final m = ref.read(topUpFlowProvider);
    if (state == PayAndPlayStage.paying || m.flow.state.value.busy) return;
    _unlisten?.call();
    state = PayAndPlayStage.paying;
    void onChange() {
      final s = m.flow.state.value;
      switch (s.stage) {
        case SettlementStage.settled:
          _stop();
          if (s.item?.status == TopUpStatus.credited) {
            state = PayAndPlayStage.idle;
            m.flow.reset();
            ref.read(playControllerProvider.notifier).start();
          } else {
            state = PayAndPlayStage.notPaid;
          }
        case SettlementStage.refused || SettlementStage.stillWaiting:
          _stop();
          state = PayAndPlayStage.notPaid;
        case SettlementStage.ready ||
            SettlementStage.sending ||
            SettlementStage.sendUncertain ||
            SettlementStage.waiting:
          break;
      }
    }

    m.flow.state.addListener(onChange);
    _unlisten = () => m.flow.state.removeListener(onChange);
    m.submit(priceKes);
  }

  /// The player has read why it was not paid; the top-up flow is free for the next request.
  void dismiss() {
    ref.read(topUpFlowProvider).flow.reset();
    state = PayAndPlayStage.idle;
  }

  void _stop() {
    _unlisten?.call();
    _unlisten = null;
  }
}

final payAndPlayProvider = NotifierProvider<PayAndPlay, PayAndPlayStage>(PayAndPlay.new);
