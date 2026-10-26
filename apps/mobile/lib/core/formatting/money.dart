/// Formats whole shillings ("KES 1,250"). Money is always an integer number of shillings (ADR 0001).
String formatKes(int amount, {bool signed = false}) {
  final digits = amount.abs().toString();
  final b = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) b.write(',');
    b.write(digits[i]);
  }
  final sign = amount < 0 ? '−' : (signed && amount > 0 ? '+' : '');
  return '${sign}KES $b';
}
