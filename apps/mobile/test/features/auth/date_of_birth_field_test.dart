import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/features/auth/presentation/date_of_birth_field.dart';

void main() {
  test('a DD/MM/YYYY date becomes the API date; anything else is not one', () {
    expect(DateOfBirthField.iso('17/05/1990'), '1990-05-17');
    expect(DateOfBirthField.iso(' 1/2/2000 '), '2000-02-01');
    expect(DateOfBirthField.iso('31/02/1990'), isNull, reason: 'no such day');
    expect(DateOfBirthField.iso('1990-05-17'), isNull);
    expect(DateOfBirthField.iso('17/05/1890'), isNull);
    expect(DateOfBirthField.iso(''), isNull);
  });

  test('under 18 is refused on the day before the 18th birthday and accepted on it (ADR 0029)', () {
    final today = DateTime(2026, 11, 7);
    expect(DateOfBirthField.error('07/11/2008', today: today), isNull);
    expect(DateOfBirthField.error('08/11/2008', today: today), 'You must be 18 or older to play.');
    expect(DateOfBirthField.error('08/11/2030', today: today), 'Enter your date of birth as DD/MM/YYYY.');
    expect(DateOfBirthField.error('not a date', today: today), 'Enter your date of birth as DD/MM/YYYY.');
  });
}
