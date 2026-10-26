import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../account/application/account_providers.dart';
import '../../draws/application/draws_providers.dart';
import '../../wallet/application/wallet_providers.dart';
import '../data/play_repository.dart';
import '../domain/play_models.dart';
import 'pending_play_store.dart';
import 'play_providers.dart';

export 'play_providers.dart' show playRepositoryProvider;

/// Where a play stands. Only the server decides an outcome; the client moves between these on its answers.
enum PlayStage {
  /// Nothing in flight.
  idle,

  /// Sent; the wheel spins without meaning while the answer is awaited.
  submitting,

  /// The server's result is in; the wheel is slowing onto its slot.
  landing,

  /// The wheel stopped; the result panel shows the server's answer.
  revealed,

  /// The server refused the play (or the session ended): nothing was charged.
  refused,

  /// No clear answer arrived (timeout, lost connection, 5xx, still in progress). The play may or may not have
  /// happened: it is checked again with the same idempotency key, which can never buy a second play.
  uncertain,
}

@immutable
class PlayFlow {
  const PlayFlow({
    this.stage = PlayStage.idle,
    this.key,
    this.result,
    this.failure,
    this.checking = false,
    this.autoChecksExhausted = false,
    this.needsConsent = false,
    this.recovered = false,
  });

  final PlayStage stage;

  /// The idempotency key of the play in hand.
  final String? key;
  final PlayResult? result;
  final ApiFailure? failure;

  /// A check with the same key is running.
  final bool checking;

  /// Automatic checks stopped; the player may check again.
  final bool autoChecksExhausted;

  /// An unfinished play from an earlier launch, too old to check without asking.
  final bool needsConsent;

  /// The play was found after an interruption (a lost answer or a restart), not answered first time.
  final bool recovered;

  bool get busy => stage == PlayStage.submitting || stage == PlayStage.landing || stage == PlayStage.uncertain;
}

/// The signed-in player's user id, from the server.
final currentUserIdProvider = FutureProvider<String>((ref) async => (await ref.watch(accountProvider.future)).userId);

/// The clock plays are timed with (overridden in tests).
final playClockProvider = Provider<DateTime Function()>((ref) => DateTime.now);

/// What to refresh once a play's outcome is shown: the day, the account and the draws. It runs when the wheel has
/// stopped (not when the answer arrives), so no list or count gives the slot away while the wheel is still turning.
final playConfirmedProvider = Provider<void Function()>(
  (ref) => () {
    ref
      ..invalidate(todayProvider)
      ..invalidate(walletStatementProvider)
      ..invalidate(accountProvider)
      ..invalidate(drawsProvider);
  },
);

/// What to refresh as soon as the server confirms a play was charged (the charge is not the outcome).
final playChargedProvider = Provider<void Function()>(
  (ref) =>
      () => ref.invalidate(walletStatementProvider),
);

final playControllerProvider = NotifierProvider<PlayController, PlayFlow>(PlayController.new);

class PlayController extends Notifier<PlayFlow> {
  /// Waits between automatic checks of an uncertain play: about half a minute in all, then the player decides.
  static const reconcileDelays = [
    Duration(seconds: 1),
    Duration(seconds: 2),
    Duration(seconds: 4),
    Duration(seconds: 8),
    Duration(seconds: 15),
  ];

  /// An unfinished play found at launch is checked by itself only if it is this recent.
  static const autoRecoverWindow = Duration(minutes: 15);

  Timer? _timer;
  int _checks = 0;

  PlayRepository get _repo => ref.read(playRepositoryProvider);
  PendingPlayStore get _store => ref.read(pendingPlayStoreProvider);
  DateTime _now() => ref.read(playClockProvider)();

  @override
  PlayFlow build() {
    ref.onDispose(() => _timer?.cancel());
    scheduleMicrotask(_recover);
    return const PlayFlow();
  }

  Future<void> _recover() async {
    final p = await _store.read();
    if (!ref.mounted || p == null || state.stage != PlayStage.idle) return;
    final String me;
    try {
      me = await ref.read(currentUserIdProvider.future);
    } on Object {
      return; // not known yet (offline, signed out): try again on the next launch
    }
    if (!ref.mounted || state.stage != PlayStage.idle) return;
    if (p.owner != me) {
      await _store.clear(); // someone else's: they will see it in their own plays when they sign in
      return;
    }
    final age = _now().difference(p.startedAt);
    if (!age.isNegative && age <= autoRecoverWindow) {
      state = PlayFlow(stage: PlayStage.uncertain, key: p.key, checking: true, recovered: true);
      await _send(p.key);
    } else {
      state = PlayFlow(stage: PlayStage.uncertain, key: p.key, needsConsent: true, recovered: true);
    }
  }

  /// Buys one play. Ignored while another is in flight, landing or unresolved.
  Future<void> start() async {
    if (state.busy) return;
    final key = newIdempotencyKey();
    _checks = 0;
    state = PlayFlow(stage: PlayStage.submitting, key: key);
    final String owner;
    try {
      owner = await ref.read(currentUserIdProvider.future);
    } on ApiFailure catch (f) {
      if (ref.mounted && state.key == key) state = PlayFlow(stage: PlayStage.refused, key: key, failure: f);
      return;
    }
    if (!ref.mounted || state.key != key) return;
    // kept before sending, so a crash between the two can never lose track of a play that may have been made
    await _store.write(PendingPlay(key: key, owner: owner, startedAt: _now()));
    await _send(key);
  }

  /// The wheel has come to rest on the result.
  void landed() {
    if (state.stage == PlayStage.landing) {
      state = PlayFlow(stage: PlayStage.revealed, key: state.key, result: state.result, recovered: state.recovered);
      ref.read(playConfirmedProvider)();
    }
  }

  /// Checks an uncertain play again, with its original key.
  Future<void> checkAgain() async {
    final key = state.key;
    if (state.stage != PlayStage.uncertain || key == null || state.checking) return;
    _timer?.cancel();
    _checks = state.needsConsent ? 0 : reconcileDelays.length;
    state = PlayFlow(stage: PlayStage.uncertain, key: key, checking: true, recovered: state.recovered);
    await _send(key);
  }

  /// Stops tracking an old unfinished play without sending it. If it did happen it shows in today's plays.
  Future<void> forget() async {
    if (state.stage != PlayStage.uncertain || !state.needsConsent) return;
    state = const PlayFlow();
    ref.read(playConfirmedProvider)();
    await _store.clear(); // last: if this is interrupted, the next launch simply asks again
  }

  void dismiss() {
    if (state.stage == PlayStage.revealed || state.stage == PlayStage.refused) state = const PlayFlow();
  }

  @visibleForTesting
  Future<void> debugDeliver(PlayResult r) => _accept(r);

  Future<void> _send(String key) async {
    try {
      final r = await _repo.play(idempotencyKey: key);
      if (!ref.mounted || state.key != key) return;
      await _accept(r);
    } on ApiFailure catch (f) {
      if (!ref.mounted || state.key != key) return;
      if (f.outcomeUncertain) {
        _uncertain(key, f);
      } else {
        await _store.clear();
        state = PlayFlow(stage: PlayStage.refused, key: key, failure: f);
      }
    }
  }

  Future<void> _accept(PlayResult r) async {
    final shown = state.result;
    if (shown != null && shown.play.ticketId == r.play.ticketId) return; // the same answer again
    final recovered = state.recovered || state.stage == PlayStage.uncertain;
    state = PlayFlow(stage: PlayStage.landing, key: state.key, result: r, recovered: recovered);
    await _store.clear();
    ref.read(playChargedProvider)();
  }

  void _uncertain(String key, ApiFailure f) {
    final more = _checks < reconcileDelays.length;
    state = PlayFlow(
      stage: PlayStage.uncertain,
      key: key,
      failure: f,
      autoChecksExhausted: !more,
      recovered: state.recovered,
    );
    if (!more) return;
    final wait = f is ProblemFailure && f.retryAfter != null && f.retryAfter! > reconcileDelays[_checks]
        ? f.retryAfter!
        : reconcileDelays[_checks];
    _checks++;
    _timer = Timer(wait, () {
      if (!ref.mounted || state.key != key || state.stage != PlayStage.uncertain) return;
      state = PlayFlow(stage: PlayStage.uncertain, key: key, failure: f, checking: true, recovered: state.recovered);
      unawaited(_send(key));
    });
  }
}
