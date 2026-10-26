/// Strict reading of API objects: a missing or mistyped required field is a [FormatException], which repositories
/// report as a malformed answer instead of showing a guessed value.
extension type JsonObject(Map<String, Object?> raw) {
  T _req<T>(String key) {
    final v = raw[key];
    if (v is T) return v;
    throw FormatException('field "$key": expected $T, got ${v.runtimeType}');
  }

  String str(String key) => _req<String>(key);
  String? strOrNull(String key) => raw[key] == null ? null : str(key);
  int integer(String key) => _req<num>(key).toInt();
  int? intOrNull(String key) => raw[key] == null ? null : integer(key);
  bool boolean(String key) => _req<bool>(key);
  DateTime instant(String key) => _parse(key, str(key));
  DateTime? instantOrNull(String key) => raw[key] == null ? null : instant(key);

  /// A calendar date ("2026-10-26"), as midnight UTC of that date.
  DateTime date(String key) {
    final s = str(key);
    final d = DateTime.tryParse(s);
    if (d == null || s.length != 10) throw FormatException('field "$key": not a date: $s');
    return DateTime.utc(d.year, d.month, d.day);
  }

  JsonObject object(String key) => JsonObject(_req<Map<String, Object?>>(key));
  JsonObject? objectOrNull(String key) => raw[key] == null ? null : object(key);

  List<T> list<T>(String key, T Function(JsonObject) item) => [
    for (final e in _req<List<Object?>>(key))
      if (e is Map<String, Object?>) item(JsonObject(e)) else throw FormatException('field "$key": item not an object'),
  ];

  static DateTime _parse(String key, String s) {
    final d = DateTime.tryParse(s);
    if (d == null) throw FormatException('field "$key": not a date-time: $s');
    return d;
  }
}
