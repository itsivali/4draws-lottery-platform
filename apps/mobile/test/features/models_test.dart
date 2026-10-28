import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/api/json.dart';
import 'package:lottery_mobile/features/account/domain/account_models.dart';
import 'package:lottery_mobile/features/draws/domain/draw_models.dart';
import 'package:lottery_mobile/features/fairness/domain/fairness_models.dart';
import 'package:lottery_mobile/features/play/domain/play_models.dart';
import 'package:lottery_mobile/features/wallet/domain/wallet_models.dart';

import '../support/fixtures.dart';

JsonObject j(String s) => JsonObject(jsonDecode(s) as Map<String, Object?>);

void main() {
  test('a play result as POST /v1/plays sends it', () {
    final r = PlayResult.fromJson(JsonObject(playResultJson(slot: 0, lucky: true, seq: 2, chances: 4, balance: 850)));
    expect(r.play.seq, 2);
    expect(r.play.spin.slot, 0);
    expect(r.play.spin.lucky, isTrue);
    expect(r.play.spin.slots, 12);
    expect(r.play.spin.luckySlots, 1);
    expect(r.play.playDate, DateTime.utc(2026, 10, 26));
    expect(r.today.chances, 4);
    expect(r.today.lucky, isTrue);
    expect(r.today.playsLeft, 2);
    expect(r.balanceKes, 850);
  });

  test('a missing required field is refused, never guessed', () {
    final bad = playResultJson()..remove('balance_kes');
    expect(() => PlayResult.fromJson(JsonObject(bad)), throwsFormatException);
    final wrongType = playResultJson()..['balance_kes'] = '850';
    expect(() => PlayResult.fromJson(JsonObject(wrongType)), throwsFormatException);
  });

  test('draws, including nulls before the freeze and results after', () {
    final open = Draw.fromJson(JsonObject(drawJson(status: 'open')));
    expect(open.type, DrawType.daily);
    expect(open.status, DrawStatus.open);
    expect(open.seedCommit, isNull);
    expect(open.beacon, isNull);
    expect(open.hasResults, isFalse);

    final done = Draw.fromJson(
      JsonObject(
        drawJson(
          status: 'approved',
          results: [
            {'rank': 1, 'amount_kes': 5000, 'entry_no': 17},
          ],
        ),
      ),
    );
    expect(done.hasResults, isTrue);
    expect(done.results.single.entryNo, 17);
    expect(done.beacon!.round, 123456);
    expect(() => Draw.fromJson(JsonObject(drawJson(status: 'mystery'))), throwsArgumentError);
  });

  test('my entries and prizes', () {
    final m = MyDrawEntries.fromJson(
      j('''{"draw_id":"d1","user_id":"u1",
      "entries":[{"entry_no":3,"play_date":"2026-10-25","ticket_id":"t1"}],
      "prizes":[{"rank":2,"amount_kes":1000,"entry_no":3,"user_id":"u1"}]}'''),
    );
    expect(m.entries.single.entryNo, 3);
    expect(m.prizes.single.amountKes, 1000);
  });

  test('wallet statement', () {
    final w = WalletStatement.fromJson(JsonObject(walletJson(balance: 850, debt: 0)));
    expect(w.balanceKes, 850);
    expect(w.transactions.first.kind, 'stake');
    expect(w.transactions.first.amountKes, -100);
    expect(w.nextBefore, isNull);
    expect(w.topUp, isNull, reason: 'no bounds when top-ups are unavailable');
    final b = WalletStatement.fromJson(
      JsonObject({
        ...walletJson(),
        'top_up': {'min_kes': 1, 'max_kes': 250000},
      }),
    ).topUp!;
    expect((b.minKes, b.maxKes), (1, 250000));
  });

  test('top-up quick picks start at the server minimum', () {
    expect(const TopUpBounds(minKes: 1, maxKes: 250000).quickPicks, [1, 100, 200, 500, 1000]);
    expect(TopUpBounds.fallback.quickPicks, [10, 100, 200, 500, 1000]);
    expect(const TopUpBounds(minKes: 300, maxKes: 250000).quickPicks, [500, 1000]);
  });

  test('withdrawal bounds come from the wallet too, and are null while withdrawals are unavailable', () {
    final w = WalletStatement.fromJson(JsonObject({...walletJson(), 'withdraw': null}));
    expect(w.withdraw, isNull);
    final b = WalletStatement.fromJson(
      JsonObject({
        ...walletJson(),
        'withdraw': {'min_kes': 20, 'max_kes': 70000},
      }),
    ).withdraw!;
    expect((b.minKes, b.maxKes), (20, 70000));
    expect(b.quickPicks, [20, 100, 500, 1000]);
    expect(WithdrawBounds.fallback.quickPicks, [50, 100, 500, 1000]);
  });

  test('top-ups and payouts', () {
    final t = TopUp.fromJson(JsonObject(topUpJson(status: 'pending')));
    expect(t.status, TopUpStatus.pending);
    expect(t.status.settled, isFalse);
    expect(t.amountConfirmedKes, isNull);
    final c = TopUp.fromJson(JsonObject(topUpJson(status: 'credited', confirmed: 500)));
    expect(c.status.settled, isTrue);
    expect(c.amountConfirmedKes, 500);

    final p = Payout.fromJson(
      j('''{"id":"p1","kind":"prize","status":"paid","amount_kes":5000,"tax_kes":1000,
      "debt_repaid_kes":0,"net_kes":4000,"receipt":"SGH12345","created_at":"2026-10-26T10:00:00+03:00"}'''),
    );
    expect(p.status, PayoutStatus.paid);
    expect(p.netKes, 4000);
  });

  test('account with a restriction and kyc in review', () {
    final a = Account.fromJson(
      JsonObject(accountJson(kyc: 'manual_review', restriction: {'kind': 'self_exclusion', 'until': null})),
    );
    expect(a.kycStatus, KycStatus.manualReview);
    expect(a.restriction!.selfExclusion, isTrue);
    expect(a.restriction!.until, isNull);
    expect(a.closureRequestedAt, isNull);
  });

  test('deposit limits with a pending increase', () {
    final l = limitsFromJson(
      j(
        '''{"limits":{"daily":{"current":{"amount_kes":500,"effective_from":"2026-10-25T00:00:00Z"},
      "pending":{"amount_kes":1000,"effective_from":"2026-10-27T00:00:00Z"}},"weekly":{"current":null,"pending":null}}}''',
      ),
    );
    expect(l['daily']!.current!.amountKes, 500);
    expect(l['daily']!.pending!.amountKes, 1000);
    expect(l['weekly']!.current, isNull);
  });

  test('fairness chains', () {
    final f = Fairness.fromJson(
      j('''{"active":{"id":"c2","commitment":"ab","client_seed":"s","next_nonce":3,
      "revealed_seed":null,"created_at":"2026-10-26T08:00:00Z","revealed_at":null},
      "rotated":[{"id":"c1","commitment":"cd","client_seed":"s","next_nonce":5,"revealed_seed":"ef",
      "created_at":"2026-10-20T08:00:00Z","revealed_at":"2026-10-26T08:00:00Z"}]}'''),
    );
    expect(f.active.revealedSeed, isNull);
    expect(f.rotated.single.revealedSeed, 'ef');
  });
}
