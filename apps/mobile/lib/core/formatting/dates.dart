// Times are shown in Nairobi time, the zone the play day, draws and limits run on (EAT, UTC+3, no daylight saving),
// whatever zone the device is set to.

const _months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const _days = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

DateTime _eat(DateTime instant) {
  final u = instant.toUtc().add(const Duration(hours: 3));
  return DateTime.utc(u.year, u.month, u.day, u.hour, u.minute, u.second);
}

String _two(int n) => n.toString().padLeft(2, '0');

/// "27 Oct 2026, 00:30 EAT".
String formatEat(DateTime instant) {
  final t = _eat(instant);
  return '${t.day} ${_months[t.month - 1]} ${t.year}, ${_two(t.hour)}:${_two(t.minute)} EAT';
}

/// "00:30" (Nairobi time).
String formatEatTime(DateTime instant) {
  final t = _eat(instant);
  return '${_two(t.hour)}:${_two(t.minute)}';
}

/// A calendar date such as a play day: "Mon 26 Oct".
String formatDay(DateTime date) => '${_days[date.weekday - 1]} ${date.day} ${_months[date.month - 1]}';

/// A draw period: "Mon 26 Oct 2026", "1 – 31 Oct 2026", "28 Sep – 4 Oct 2026", "1 Oct – 31 Dec 2026".
String formatPeriod(DateTime start, DateTime end) {
  if (start == end) return '${formatDay(start)} ${start.year}';
  final endText = '${end.day} ${_months[end.month - 1]} ${end.year}';
  if (start.year == end.year && start.month == end.month) return '${start.day} – $endText';
  final startText = start.year == end.year
      ? '${start.day} ${_months[start.month - 1]}'
      : '${start.day} ${_months[start.month - 1]} ${start.year}';
  return '$startText – $endText';
}
