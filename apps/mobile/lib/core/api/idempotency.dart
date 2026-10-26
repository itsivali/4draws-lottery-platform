import 'dart:math';

final _random = Random.secure();

/// A fresh key for one logical write (the API requires one on every POST, PUT, PATCH and DELETE). Keep it for as long
/// as the outcome is unknown: retrying with the same key can never repeat the effect.
String newIdempotencyKey() {
  final b = StringBuffer('app-');
  for (var i = 0; i < 16; i++) {
    b.write(_random.nextInt(256).toRadixString(16).padLeft(2, '0'));
  }
  return b.toString();
}
