import 'dart:async';

import 'package:flutter/foundation.dart';

import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';

enum SettlementStage {
  /// Nothing sent yet.
  ready,

  /// The request is on its way.
  sending,

  /// The request's answer was lost; it is retried with the same key (the API replays, never repeats).
  sendUncertain,

  /// The API refused the request: nothing was started.
  refused,

  /// Accepted and waiting for the server to confirm the money moved.
  waiting,

  /// Automatic checks stopped; the item is still unsettled on the server. Safe to leave and come back.
  stillWaiting,

  /// The server reports a final state (credited / failed / reversed, or paid / failed).
  settled,
}

@immutable
class SettlementState<T> {
  const SettlementState(this.stage, {this.item, this.failure, this.checking = false});

  final SettlementStage stage;
  final T? item;
  final ApiFailure? failure;
  final bool checking;

  bool get busy =>
      stage == SettlementStage.sending || stage == SettlementStage.waiting || stage == SettlementStage.sendUncertain;
}

/// One money request (a top-up or a withdrawal) from sending to the server's final answer. The server is the only
/// source of truth: a returned prompt, an elapsed timer or a callback the app saw prove nothing. Polling backs off and
/// stops after [maxWait]; the player can check again, or leave and come back (the server lists unsettled items).
class SettlementFlow<T> {
  SettlementFlow({
    required this.send,
    required this.fetch,
    required this.idOf,
    required this.settled,
    this.maxWait = const Duration(minutes: 3),
  });

  static const sendRetries = [Duration(seconds: 1), Duration(seconds: 3), Duration(seconds: 6)];
  static const pollDelays = [
    Duration(seconds: 2),
    Duration(seconds: 3),
    Duration(seconds: 5),
    Duration(seconds: 8),
    Duration(seconds: 13),
    Duration(seconds: 15),
  ];

  final Future<T> Function(String idempotencyKey) send;
  final Future<T> Function(String id) fetch;
  final String Function(T) idOf;
  final bool Function(T) settled;
  final Duration maxWait;

  /// Called once when the item reaches a final state.
  void Function(T item)? onSettled;

  final state = ValueNotifier(SettlementState<T>(SettlementStage.ready));

  String? _key;
  Timer? _timer;
  int _sendTries = 0;
  int _polls = 0;
  Duration _waited = Duration.zero;
  bool _disposed = false;
  bool _reportedSettled = false;

  /// Sends the request once. Ignored while a request is unresolved or already followed.
  void submit() {
    if (state.value.stage != SettlementStage.ready && state.value.stage != SettlementStage.refused) return;
    _key = newIdempotencyKey();
    _sendTries = 0;
    _set(SettlementState(SettlementStage.sending));
    unawaited(_send());
  }

  /// Follows an item already sent (found pending in the server's list).
  void track(T item) {
    if (state.value.busy) return;
    _accepted(item);
  }

  /// Sends again with the same key after the answer was lost.
  void checkSend() {
    if (state.value.stage != SettlementStage.sendUncertain || state.value.checking) return;
    _sendTries = sendRetries.length;
    _set(SettlementState(SettlementStage.sendUncertain, failure: state.value.failure, checking: true));
    unawaited(_send());
  }

  /// Asks the server for the latest status now.
  void refresh() {
    final item = state.value.item;
    if (item == null || state.value.checking) return;
    _timer?.cancel();
    unawaited(_poll(idOf(item), manual: true));
  }

  Future<void> _send() async {
    final key = _key!;
    try {
      final item = await send(key);
      if (_disposed || key != _key) return;
      _accepted(item);
    } on ApiFailure catch (f) {
      if (_disposed || key != _key) return;
      if (!f.outcomeUncertain) {
        _set(SettlementState(SettlementStage.refused, failure: f));
        return;
      }
      if (_sendTries >= sendRetries.length) {
        _set(SettlementState(SettlementStage.sendUncertain, failure: f));
        return;
      }
      _set(SettlementState(SettlementStage.sendUncertain, failure: f, checking: true));
      _timer = Timer(sendRetries[_sendTries++], () => unawaited(_send()));
    }
  }

  void _accepted(T item) {
    _polls = 0;
    _waited = Duration.zero;
    if (settled(item)) {
      _settle(item);
      return;
    }
    _set(SettlementState(SettlementStage.waiting, item: item));
    _schedule(idOf(item));
  }

  void _schedule(String id) {
    final wait = pollDelays[_polls < pollDelays.length ? _polls : pollDelays.length - 1];
    if (_waited + wait > maxWait) {
      _set(SettlementState(SettlementStage.stillWaiting, item: state.value.item));
      return;
    }
    _polls++;
    _waited += wait;
    _timer = Timer(wait, () => unawaited(_poll(id)));
  }

  Future<void> _poll(String id, {bool manual = false}) async {
    if (manual) _set(SettlementState(state.value.stage, item: state.value.item, checking: true));
    try {
      final item = await fetch(id);
      if (_disposed) return;
      if (settled(item)) {
        _settle(item);
        return;
      }
      if (manual) {
        _polls = 0;
        _waited = Duration.zero;
      }
      _set(SettlementState(SettlementStage.waiting, item: item));
    } on ApiFailure {
      if (_disposed) return;
      _set(
        SettlementState(
          state.value.stage == SettlementStage.stillWaiting && !manual
              ? SettlementStage.stillWaiting
              : SettlementStage.waiting,
          item: state.value.item,
        ),
      );
    }
    _schedule(id);
  }

  void _settle(T item) {
    _timer?.cancel();
    _set(SettlementState(SettlementStage.settled, item: item));
    if (_reportedSettled) return;
    _reportedSettled = true;
    onSettled?.call(item);
  }

  void _set(SettlementState<T> s) {
    if (!_disposed) state.value = s;
  }

  void dispose() {
    _disposed = true;
    _timer?.cancel();
    state.dispose();
  }
}
