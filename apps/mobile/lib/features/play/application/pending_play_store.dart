import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// A play whose outcome the app has not learned yet: its idempotency key (not a secret: it only works with the
/// player's own token), whose play it is, and when it was sent.
class PendingPlay {
  const PendingPlay({required this.key, required this.owner, required this.startedAt});

  final String key;

  /// The player's user id. Keys are scoped per player, so a key replayed under someone else's session would buy them
  /// a new play: it is only ever checked for its owner.
  final String owner;
  final DateTime startedAt;
}

abstract interface class PendingPlayStore {
  Future<PendingPlay?> read();
  Future<void> write(PendingPlay p);
  Future<void> clear();
}

/// One JSON value, written in one call: a crash mid-write leaves either the old record or the new one, never a
/// half-written record that reads as nothing.
class PrefsPendingPlayStore implements PendingPlayStore {
  const PrefsPendingPlayStore(this._prefs);

  static const _record = 'pending_play';

  // the three keys written before the record was one value; read once more, then superseded
  static const _legacyKey = 'pending_play.key',
      _legacyOwner = 'pending_play.owner',
      _legacyAt = 'pending_play.started_at';
  final SharedPreferencesAsync _prefs;

  @override
  Future<PendingPlay?> read() async {
    final raw = await _prefs.getString(_record);
    if (raw != null) {
      try {
        final j = jsonDecode(raw) as Map<String, Object?>;
        final key = j['key'], owner = j['owner'], at = DateTime.tryParse(j['started_at'] as String? ?? '');
        if (key is String && owner is String && at != null) return PendingPlay(key: key, owner: owner, startedAt: at);
      } on FormatException {
        // unreadable: treated as absent below
      }
      return null;
    }
    final key = await _prefs.getString(_legacyKey);
    final owner = await _prefs.getString(_legacyOwner);
    final at = DateTime.tryParse(await _prefs.getString(_legacyAt) ?? '');
    return key == null || owner == null || at == null ? null : PendingPlay(key: key, owner: owner, startedAt: at);
  }

  @override
  Future<void> write(PendingPlay p) async {
    await _prefs.setString(
      _record,
      jsonEncode({'key': p.key, 'owner': p.owner, 'started_at': p.startedAt.toUtc().toIso8601String()}),
    );
    await _clearLegacy();
  }

  @override
  Future<void> clear() async {
    await _prefs.remove(_record);
    await _clearLegacy();
  }

  Future<void> _clearLegacy() async {
    await _prefs.remove(_legacyKey);
    await _prefs.remove(_legacyOwner);
    await _prefs.remove(_legacyAt);
  }
}

class MemoryPendingPlayStore implements PendingPlayStore {
  PendingPlay? value;

  @override
  Future<PendingPlay?> read() async => value;

  @override
  Future<void> write(PendingPlay p) async => value = p;

  @override
  Future<void> clear() async => value = null;
}

final pendingPlayStoreProvider = Provider<PendingPlayStore>((ref) => PrefsPendingPlayStore(SharedPreferencesAsync()));
