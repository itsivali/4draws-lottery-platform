import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'money_flows.dart';
import 'settlement_flow.dart';
import 'wallet_providers.dart';

/// When the player may next ask for an M-Pesa prompt, or null when they may now (owner, 2026-10-30: say how long, so
/// prompts are not asked for faster than Daraja answers). The server decides and refuses one asked too soon; this
/// only lets the app say so up front. It follows the wallet's `wait_s`, and starts the full cooldown the moment the
/// server accepts a top-up.
class TopUpCooldown extends Notifier<DateTime?> {
  @override
  DateTime? build() {
    final bounds = ref.watch(walletStatementProvider).value?.topUp;
    final flow = ref.watch(topUpFlowProvider).flow;
    var last = flow.state.value.stage;
    void onChange() {
      final s = flow.state.value;
      if (last != SettlementStage.waiting && s.stage == SettlementStage.waiting) {
        final cooldown = ref.read(walletStatementProvider).value?.topUp?.cooldownS ?? 0;
        if (cooldown > 0) state = DateTime.now().add(Duration(seconds: cooldown));
      }
      if (s.stage == SettlementStage.refused) ref.invalidate(walletStatementProvider); // learn the server's own wait
      last = s.stage;
    }

    flow.state.addListener(onChange);
    ref.onDispose(() => flow.state.removeListener(onChange));
    if (bounds == null || bounds.waitS <= 0) return null;
    return DateTime.now().add(Duration(seconds: bounds.waitS));
  }
}

final topUpCooldownProvider = NotifierProvider<TopUpCooldown, DateTime?>(TopUpCooldown.new);

/// "2 minutes", "1 minute", "45 seconds": how long is left, rounded up to the minute above a minute.
String describeWait(Duration left) {
  final s = left.inSeconds + (left.inMilliseconds % 1000 > 0 ? 1 : 0);
  if (s <= 60) return s == 1 ? '1 second' : '$s seconds';
  final m = (s + 59) ~/ 60;
  return m == 1 ? '1 minute' : '$m minutes';
}
