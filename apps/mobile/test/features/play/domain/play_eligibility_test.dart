import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/api/json.dart';
import 'package:lottery_mobile/features/account/domain/account_models.dart';
import 'package:lottery_mobile/features/play/domain/play_eligibility.dart';
import 'package:lottery_mobile/features/play/domain/play_models.dart';
import 'package:lottery_mobile/features/wallet/domain/wallet_models.dart';

import '../../../support/fixtures.dart';

Account account({
  String kyc = 'verified',
  bool phone = true,
  String status = 'active',
  Map<String, Object?>? restriction,
}) =>
    Account.fromJson(JsonObject(accountJson(kyc: kyc, phoneVerified: phone, status: status, restriction: restriction)));
Today today({int plays = 0}) => Today.fromJson(JsonObject(todayJson(plays: plays)));
WalletStatement wallet({int balance = 850, int debt = 0}) =>
    WalletStatement.fromJson(JsonObject(walletJson(balance: balance, debt: debt)));

void main() {
  // the fixtures' restriction ends on 2026-10-27; the test's 'now' is the day before
  final now = DateTime.utc(2026, 10, 26);
  PlayBlock? block({Account? a, Today? t, WalletStatement? w}) =>
      playBlock(account: a ?? account(), today: t ?? today(), wallet: w ?? wallet(), now: now);

  test('an eligible player with plays left and enough balance may play', () {
    expect(block(), isNull);
  });

  test('each server restriction blocks with its own reason', () {
    expect(block(a: account(status: 'suspended'))!.reason, PlayBlockReason.suspended);
    expect(block(a: account(status: 'closed'))!.reason, PlayBlockReason.closed);
    expect(
      block(a: account(restriction: {'kind': 'timeout', 'until': '2026-10-27T09:00:00Z'}))!.reason,
      PlayBlockReason.timedOut,
    );
    expect(
      block(a: account(restriction: {'kind': 'timeout', 'until': '2026-10-27T09:00:00Z'}))!.until,
      DateTime.utc(2026, 10, 27, 9),
    );
    expect(
      block(a: account(restriction: {'kind': 'self_exclusion', 'until': null}))!.reason,
      PlayBlockReason.selfExcluded,
    );
    expect(block(a: account(phone: false))!.reason, PlayBlockReason.phoneNotProved);
    expect(block(w: wallet(debt: 30))!.reason, PlayBlockReason.inDebt);
    expect(block(t: today(plays: 4))!.reason, PlayBlockReason.dailyLimit);
    expect(block(w: wallet(balance: 99))!.reason, PlayBlockReason.lowBalance);
  });

  test('the most fundamental reason wins (a self-excluded player is never told to top up)', () {
    final r = block(
      a: account(kyc: 'none', phone: false, restriction: {'kind': 'self_exclusion', 'until': null}),
      t: today(plays: 4),
      w: wallet(balance: 0, debt: 10),
    );
    expect(r!.reason, PlayBlockReason.selfExcluded);
    expect(block(a: account(phone: false), w: wallet(balance: 0))!.reason, PlayBlockReason.phoneNotProved);
    expect(block(t: today(plays: 4), w: wallet(balance: 0))!.reason, PlayBlockReason.dailyLimit);
  });

  test('the price comes from the server, not from the app', () {
    final t = Today.fromJson(JsonObject({...todayJson(), 'price_kes': 150}));
    expect(playBlock(account: account(), today: t, wallet: wallet(balance: 120))!.reason, PlayBlockReason.lowBalance);
    expect(playBlock(account: account(), today: t, wallet: wallet(balance: 150)), isNull);
  });

  test('a break whose end has passed no longer blocks, even before the account is re-read', () {
    final paused = account(restriction: {'kind': 'timeout', 'until': '2026-10-27T09:00:00Z'});
    expect(playBlock(account: paused, today: today(), wallet: wallet(), now: DateTime.utc(2026, 10, 28)), isNull);
    expect(playBlock(account: paused, today: today(), wallet: wallet(), now: now)!.reason, PlayBlockReason.timedOut);
  });
}
