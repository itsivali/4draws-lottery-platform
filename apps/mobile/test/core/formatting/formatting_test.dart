import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/core/formatting/dates.dart';
import 'package:lottery_mobile/core/formatting/money.dart';
import 'package:lottery_mobile/core/formatting/phone.dart';

void main() {
  test('KES amounts are whole shillings with thousands separators', () {
    expect(formatKes(0), 'KES 0');
    expect(formatKes(100), 'KES 100');
    expect(formatKes(1250), 'KES 1,250');
    expect(formatKes(150000), 'KES 150,000');
    expect(formatKes(1234567), 'KES 1,234,567');
    expect(formatKes(-100), '−KES 100');
  });

  test('signed amounts show the direction of a wallet movement', () {
    expect(formatKes(950, signed: true), '+KES 950');
    expect(formatKes(-100, signed: true), '−KES 100');
  });

  test('times are shown in Nairobi time (EAT, UTC+3, no daylight saving), whatever the device zone', () {
    final t = DateTime.utc(2026, 10, 26, 21, 30);
    expect(formatEat(t), '27 Oct 2026, 00:30 EAT');
    expect(formatEatTime(t), '00:30');
    expect(formatEat(DateTime.parse('2026-10-26T10:05:00+03:00')), '26 Oct 2026, 10:05 EAT');
  });

  test('calendar dates (play days, draw periods) are not shifted by time zones', () {
    expect(formatDay(DateTime.utc(2026, 10, 26)), 'Mon 26 Oct');
    expect(formatPeriod(DateTime.utc(2026, 10, 1), DateTime.utc(2026, 10, 31)), '1 – 31 Oct 2026');
    expect(formatPeriod(DateTime.utc(2026, 10, 26), DateTime.utc(2026, 10, 26)), 'Mon 26 Oct 2026');
    expect(formatPeriod(DateTime.utc(2026, 9, 28), DateTime.utc(2026, 10, 4)), '28 Sep – 4 Oct 2026');
    expect(formatPeriod(DateTime.utc(2026, 10, 1), DateTime.utc(2026, 12, 31)), '1 Oct – 31 Dec 2026');
  });

  test('phone numbers are shown the way Kenyans write them', () {
    expect(formatMsisdn('254712345678'), '0712 345 678');
    expect(formatMsisdn('254112345678'), '0112 345 678');
    expect(formatMsisdn('weird'), 'weird');
  });

  test('phone input is normalised the way the API accepts it', () {
    expect(isLikelyKenyanMobile('0712345678'), isTrue);
    expect(isLikelyKenyanMobile('+254 712 345 678'), isTrue);
    expect(isLikelyKenyanMobile('254112345678'), isTrue);
    expect(isLikelyKenyanMobile('0612345678'), isFalse);
    expect(isLikelyKenyanMobile('12345'), isFalse);
  });
}
