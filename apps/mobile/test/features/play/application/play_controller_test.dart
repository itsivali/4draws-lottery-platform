import 'dart:async';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/api/api_failure.dart';
import 'package:lottery_mobile/core/api/json.dart';
import 'package:lottery_mobile/features/play/application/pending_play_store.dart';
import 'package:lottery_mobile/features/play/application/play_controller.dart';
import 'package:lottery_mobile/features/play/data/play_repository.dart';
import 'package:lottery_mobile/features/play/domain/play_models.dart';

import '../../../support/fixtures.dart';

/// A scripted server: each call to play() takes the next answer.
class ScriptedPlays implements PlayRepository {
  final answers = <FutureOr<PlayResult> Function()>[];
  final keys = <String>[];

  void answer(PlayResult r) => answers.add(() => r);
  void fail(ApiFailure f) => answers.add(() => throw f);
  void hang(Completer<PlayResult> c) => answers.add(() => c.future);

  @override
  Future<PlayResult> play({required String idempotencyKey}) async {
    keys.add(idempotencyKey);
    if (answers.isEmpty) throw StateError('unexpected play call');
    return answers.removeAt(0)();
  }

  @override
  Future<Today> today() => throw UnimplementedError();
}

PlayResult result({
  String ticket = 'ticket-1',
  int slot = 6,
  bool lucky = false,
  int seq = 1,
  int chances = 1,
  bool replayed = false,
}) => PlayResult.fromJson(
  JsonObject(playResultJson(ticket: ticket, slot: slot, lucky: lucky, seq: seq, chances: chances)),
  replayed: replayed,
);

const timeout = NetworkFailure(timedOut: true);
const dailyLimit = ProblemFailure(status: 409, slug: 'daily-limit', title: 'No plays left today');

void main() {
  late ScriptedPlays server;
  late MemoryPendingPlayStore pending;
  late DateTime now;
  late int confirmations;
  late int charges;
  late String currentUser;

  ProviderContainer container() {
    final c = ProviderContainer.test(
      overrides: [
        playRepositoryProvider.overrideWithValue(server),
        pendingPlayStoreProvider.overrideWithValue(pending),
        playClockProvider.overrideWithValue(() => now),
        playConfirmedProvider.overrideWithValue(() => confirmations++),
        playChargedProvider.overrideWithValue(() => charges++),
        currentUserIdProvider.overrideWith((ref) async => currentUser),
      ],
      retry: (_, _) => null,
    );
    c.listen(playControllerProvider, (_, _) {}); // keep it alive, like the screen does
    return c;
  }

  setUp(() {
    server = ScriptedPlays();
    pending = MemoryPendingPlayStore();
    now = DateTime.utc(2026, 10, 26, 9);
    confirmations = 0;
    charges = 0;
    currentUser = 'user-1';
  });

  test('a successful play goes submitting → landing → revealed, with the server result', () {
    fakeAsync((async) {
      final c = container();
      final ctl = c.read(playControllerProvider.notifier);
      server.answer(result(slot: 3, chances: 1));
      ctl.start();
      expect(c.read(playControllerProvider).stage, PlayStage.submitting);
      async.flushMicrotasks();
      final s = c.read(playControllerProvider);
      expect(s.stage, PlayStage.landing);
      expect(s.result!.play.spin.slot, 3);
      expect(pending.value, isNull);
      expect(charges, 1, reason: 'the wallet is refreshed as soon as the charge is confirmed');
      expect(confirmations, 0, reason: "the day's numbers wait for the wheel, so nothing gives the slot away early");
      ctl.landed();
      expect(c.read(playControllerProvider).stage, PlayStage.revealed);
      expect(confirmations, 1);
    });
  });

  test('tapping again while a play is in flight starts nothing', () {
    fakeAsync((async) {
      final c = container();
      final ctl = c.read(playControllerProvider.notifier);
      final slow = Completer<PlayResult>();
      server.hang(slow);
      ctl.start();
      ctl.start();
      ctl.start();
      async.flushMicrotasks();
      expect(server.keys, hasLength(1));
      slow.complete(result());
      async.flushMicrotasks();
      ctl.start(); // still landing: the wheel has not stopped
      async.flushMicrotasks();
      expect(server.keys, hasLength(1));
    });
  });

  test('a refusal is shown as the server said it, and nothing spins to an outcome', () {
    fakeAsync((async) {
      final c = container();
      server.fail(dailyLimit);
      c.read(playControllerProvider.notifier).start();
      async.flushMicrotasks();
      final s = c.read(playControllerProvider);
      expect(s.stage, PlayStage.refused);
      expect((s.failure! as ProblemFailure).slug, 'daily-limit');
      expect(s.result, isNull);
      expect(pending.value, isNull);
      expect(confirmations, 0);
    });
  });

  for (final f in const [
    ProblemFailure(status: 403, slug: 'kyc-required', title: 'Identity verification required'),
    ProblemFailure(status: 403, slug: 'restricted', title: 'Your account is restricted'),
    ProblemFailure(status: 403, slug: 'in-debt', title: 'Play is paused'),
    ProblemFailure(status: 409, slug: 'insufficient-funds', title: 'Your balance is below the price of a play'),
    SessionEndedFailure(),
  ]) {
    test('$f is a definite refusal', () {
      fakeAsync((async) {
        final c = container();
        server.fail(f);
        c.read(playControllerProvider.notifier).start();
        async.flushMicrotasks();
        expect(c.read(playControllerProvider).stage, PlayStage.refused);
        expect(server.keys, hasLength(1));
      });
    });
  }

  test('a timeout is uncertain: it is checked again with the same key, and the real result recovered', () {
    fakeAsync((async) {
      final c = container();
      server.fail(timeout);
      server.fail(const NetworkFailure());
      server.answer(result(slot: 9, replayed: true));
      c.read(playControllerProvider.notifier).start();
      async.flushMicrotasks();
      var s = c.read(playControllerProvider);
      expect(s.stage, PlayStage.uncertain);
      expect(s.result, isNull, reason: 'no outcome is invented while unsure');
      async.elapse(const Duration(seconds: 1));
      expect(c.read(playControllerProvider).stage, PlayStage.uncertain);
      async.elapse(const Duration(seconds: 2));
      s = c.read(playControllerProvider);
      expect(s.stage, PlayStage.landing);
      expect(s.result!.play.spin.slot, 9);
      expect(
        server.keys.toSet(),
        hasLength(1),
        reason: 'every check reuses the first key, so it can never buy a second play',
      );
      expect(pending.value, isNull);
    });
  });

  test('a play still being processed (409 request-in-progress) is checked again, not failed', () {
    fakeAsync((async) {
      final c = container();
      server.fail(const ProblemFailure(status: 409, slug: 'request-in-progress', title: 'Request in progress'));
      server.answer(result());
      c.read(playControllerProvider.notifier).start();
      async.flushMicrotasks();
      expect(c.read(playControllerProvider).stage, PlayStage.uncertain);
      async.elapse(const Duration(seconds: 1));
      expect(c.read(playControllerProvider).stage, PlayStage.landing);
    });
  });

  test('automatic checks stop after a bounded number; the player can check again with the same key', () {
    fakeAsync((async) {
      final c = container();
      for (var i = 0; i < 1 + PlayController.reconcileDelays.length; i++) {
        server.fail(timeout);
      }
      c.read(playControllerProvider.notifier).start();
      async.elapse(const Duration(minutes: 5));
      final s = c.read(playControllerProvider);
      expect(s.stage, PlayStage.uncertain);
      expect(s.autoChecksExhausted, isTrue);
      expect(server.keys, hasLength(1 + PlayController.reconcileDelays.length));
      expect(pending.value, isNotNull, reason: 'kept so a later launch can still find out');

      server.answer(result(ticket: 'ticket-7'));
      c.read(playControllerProvider.notifier).checkAgain();
      async.flushMicrotasks();
      expect(c.read(playControllerProvider).result!.play.ticketId, 'ticket-7');
      expect(server.keys.toSet(), hasLength(1));
    });
  });

  test('a definite refusal while reconciling ends the uncertainty', () {
    fakeAsync((async) {
      final c = container();
      server.fail(timeout);
      server.fail(
        const ProblemFailure(
          status: 409,
          slug: 'insufficient-funds',
          title: 'Your balance is below the price of a play',
        ),
      );
      c.read(playControllerProvider.notifier).start();
      async.elapse(const Duration(seconds: 2));
      expect(c.read(playControllerProvider).stage, PlayStage.refused);
      expect(pending.value, isNull);
    });
  });

  test('the same result delivered twice is shown once', () {
    fakeAsync((async) {
      final c = container();
      final ctl = c.read(playControllerProvider.notifier);
      server.answer(result(ticket: 't-1'));
      ctl.start();
      async.flushMicrotasks();
      final first = c.read(playControllerProvider);
      ctl.debugDeliver(result(ticket: 't-1', replayed: true));
      expect(identical(c.read(playControllerProvider), first), isTrue);
      expect(charges, 1);
    });
  });

  test('a result arriving after the controller is gone changes nothing and does not throw', () {
    fakeAsync((async) {
      final c = container();
      final slow = Completer<PlayResult>();
      server.hang(slow);
      c.read(playControllerProvider.notifier).start();
      async.flushMicrotasks();
      c.dispose();
      slow.complete(result());
      async.flushMicrotasks();
      expect(confirmations + charges, 0);
      expect(pending.value, isNotNull, reason: 'the next launch reconciles it');
    });
  });

  test('at launch, a recent unfinished play is reconciled automatically with its own key', () {
    fakeAsync((async) {
      pending.value = PendingPlay(
        key: 'app-old-key',
        owner: 'user-1',
        startedAt: now.subtract(const Duration(minutes: 3)),
      );
      server.answer(result(ticket: 'recovered', replayed: true));
      final c = container();
      async.flushMicrotasks();
      final s = c.read(playControllerProvider);
      expect(server.keys, ['app-old-key']);
      expect(s.stage, PlayStage.landing);
      expect(s.result!.play.ticketId, 'recovered');
      expect(s.recovered, isTrue);
    });
  });

  test('an old unfinished play is not sent again without the player saying so', () {
    fakeAsync((async) {
      pending.value = PendingPlay(
        key: 'app-old-key',
        owner: 'user-1',
        startedAt: now.subtract(const Duration(hours: 5)),
      );
      final c = container();
      async.flushMicrotasks();
      final s = c.read(playControllerProvider);
      expect(s.stage, PlayStage.uncertain);
      expect(s.needsConsent, isTrue);
      expect(server.keys, isEmpty);

      c.read(playControllerProvider.notifier).forget();
      expect(c.read(playControllerProvider).stage, PlayStage.idle);
      expect(pending.value, isNull);
      expect(confirmations, 1, reason: "today's plays are reloaded so a play that did happen shows up");
    });
  });

  test('after a result or refusal the player can play again with a new key', () {
    fakeAsync((async) {
      final c = container();
      final ctl = c.read(playControllerProvider.notifier);
      server.fail(dailyLimit);
      ctl.start();
      async.flushMicrotasks();
      ctl.dismiss();
      server.answer(result());
      ctl.start();
      async.flushMicrotasks();
      expect(server.keys.toSet(), hasLength(2));
    });
  });

  test("another player's unfinished play is never sent under this player's session", () {
    fakeAsync((async) {
      pending.value = PendingPlay(
        key: 'app-their-key',
        owner: 'user-2',
        startedAt: now.subtract(const Duration(minutes: 1)),
      );
      final c = container();
      async.flushMicrotasks();
      expect(
        server.keys,
        isEmpty,
        reason: 'keys are per player: replaying it here would buy a new play for this player',
      );
      expect(c.read(playControllerProvider).stage, PlayStage.idle);
      expect(pending.value, isNull);
    });
  });

  test('the play remembers whose it is', () {
    fakeAsync((async) {
      final c = container();
      server.hang(Completer<PlayResult>());
      c.read(playControllerProvider.notifier).start();
      async.flushMicrotasks();
      expect(pending.value!.owner, 'user-1');
    });
  });
}
