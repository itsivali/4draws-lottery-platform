import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/api/api_failure.dart';
import 'package:lottery_mobile/core/api/json.dart';
import 'package:lottery_mobile/features/wallet/application/settlement_flow.dart';
import 'package:lottery_mobile/features/wallet/domain/wallet_models.dart';

import '../../support/fixtures.dart';

TopUp topUp(String status, {int? confirmed}) =>
    TopUp.fromJson(JsonObject(topUpJson(status: status, confirmed: confirmed)));

void main() {
  late List<String> sendKeys;
  late List<Object> sendAnswers; // TopUp or ApiFailure, in order
  late List<Object> statusAnswers;
  late int fetches;

  SettlementFlow<TopUp> flow() => SettlementFlow<TopUp>(
    send: (key) async {
      sendKeys.add(key);
      final a = sendAnswers.removeAt(0);
      if (a is ApiFailure) throw a;
      return a as TopUp;
    },
    fetch: (id) async {
      fetches++;
      final a = statusAnswers.isEmpty ? topUp('pending') : statusAnswers.removeAt(0);
      if (a is ApiFailure) throw a;
      return a as TopUp;
    },
    idOf: (t) => t.id,
    settled: (t) => t.status.settled,
  );

  setUp(() {
    sendKeys = [];
    sendAnswers = [];
    statusAnswers = [];
    fetches = 0;
  });

  test('a top-up is pending until the server says credited; nothing is assumed from time passing', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.add(topUp('pending'));
      statusAnswers.addAll([topUp('pending'), topUp('pending'), topUp('credited', confirmed: 500)]);
      f.submit();
      async.flushMicrotasks();
      expect(f.state.value.stage, SettlementStage.waiting);
      expect(f.state.value.item!.status, TopUpStatus.pending);
      async.elapse(const Duration(seconds: 2));
      expect(f.state.value.stage, SettlementStage.waiting);
      async.elapse(const Duration(seconds: 10));
      expect(f.state.value.stage, SettlementStage.settled);
      expect(f.state.value.item!.status, TopUpStatus.credited);
      final n = fetches;
      async.elapse(const Duration(minutes: 5));
      expect(fetches, n, reason: 'polling stops once settled');
      f.dispose();
    });
  });

  test('polling is bounded: after a few minutes it stops and says it is still waiting', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.add(topUp('pending'));
      f.submit();
      async.elapse(const Duration(minutes: 10));
      expect(f.state.value.stage, SettlementStage.stillWaiting);
      expect(fetches, lessThanOrEqualTo(20), reason: 'no tight loop');
      final n = fetches;
      async.elapse(const Duration(hours: 1));
      expect(fetches, n);
      statusAnswers.add(topUp('failed'));
      f.refresh();
      async.flushMicrotasks();
      expect(f.state.value.stage, SettlementStage.settled);
      expect(f.state.value.item!.status, TopUpStatus.failed);
      f.dispose();
    });
  });

  test('a lost answer to the request is retried with the same key, then left for the player to check', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.addAll([
        const NetworkFailure(timedOut: true),
        const NetworkFailure(),
        const NetworkFailure(),
        const NetworkFailure(),
      ]);
      f.submit();
      async.elapse(const Duration(minutes: 1));
      expect(f.state.value.stage, SettlementStage.sendUncertain);
      expect(sendKeys.toSet(), hasLength(1), reason: 'a retried key is replayed by the API, never a second payment');
      sendAnswers.add(topUp('pending'));
      f.checkSend();
      async.flushMicrotasks();
      expect(f.state.value.stage, SettlementStage.waiting);
      expect(sendKeys.toSet(), hasLength(1));
      f.dispose();
    });
  });

  test('a refusal is final and nothing is polled', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.add(
        const ProblemFailure(
          status: 403,
          slug: 'deposit-limit',
          title: 'This top-up would exceed your daily deposit limit',
        ),
      );
      f.submit();
      async.elapse(const Duration(minutes: 1));
      expect(f.state.value.stage, SettlementStage.refused);
      expect((f.state.value.failure! as ProblemFailure).slug, 'deposit-limit');
      expect(fetches, 0);
      f.dispose();
    });
  });

  test('a second submit while one is in flight is ignored', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.add(topUp('pending'));
      f.submit();
      f.submit();
      async.flushMicrotasks();
      expect(sendKeys, hasLength(1));
      f.submit();
      async.flushMicrotasks();
      expect(sendKeys, hasLength(1), reason: 'never start a second payment while the first is unresolved');
      f.dispose();
    });
  });

  test('the same status arriving again does not count as a change', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.add(topUp('pending'));
      statusAnswers.addAll([topUp('credited', confirmed: 500), topUp('credited', confirmed: 500)]);
      var settledEvents = 0;
      f.onSettled = (_) => settledEvents++;
      f.submit();
      async.elapse(const Duration(seconds: 30));
      f.refresh();
      async.flushMicrotasks();
      expect(settledEvents, 1);
      f.dispose();
    });
  });

  test('a network error while following the status keeps following, not failing', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.add(topUp('pending'));
      statusAnswers.addAll([const NetworkFailure(), topUp('credited', confirmed: 500)]);
      f.submit();
      async.elapse(const Duration(seconds: 10));
      expect(f.state.value.stage, SettlementStage.settled);
      f.dispose();
    });
  });

  test('tracking an existing pending item (after a restart) resumes polling', () {
    fakeAsync((async) {
      final f = flow();
      statusAnswers.add(topUp('credited', confirmed: 500));
      f.track(topUp('pending'));
      async.elapse(const Duration(seconds: 3));
      expect(f.state.value.stage, SettlementStage.settled);
      expect(sendKeys, isEmpty);
      f.dispose();
    });
  });

  test('disposing stops everything', () {
    fakeAsync((async) {
      final f = flow();
      sendAnswers.add(topUp('pending'));
      f.submit();
      async.flushMicrotasks();
      f.dispose();
      final n = fetches;
      async.elapse(const Duration(minutes: 5));
      expect(fetches, n);
    });
  });
}
