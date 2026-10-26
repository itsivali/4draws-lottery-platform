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

class PrefsPendingPlayStore implements PendingPlayStore {
  const PrefsPendingPlayStore(this._prefs);

  static const _key = 'pending_play.key', _owner = 'pending_play.owner', _at = 'pending_play.started_at';
  final SharedPreferencesAsync _prefs;

  @override
  Future<PendingPlay?> read() async {
    final key = await _prefs.getString(_key);
    final owner = await _prefs.getString(_owner);
    final at = DateTime.tryParse(await _prefs.getString(_at) ?? '');
    return key == null || owner == null || at == null ? null : PendingPlay(key: key, owner: owner, startedAt: at);
  }

  @override
  Future<void> write(PendingPlay p) async {
    await _prefs.setString(_key, p.key);
    await _prefs.setString(_owner, p.owner);
    await _prefs.setString(_at, p.startedAt.toUtc().toIso8601String());
  }

  @override
  Future<void> clear() async {
    await _prefs.remove(_key);
    await _prefs.remove(_owner);
    await _prefs.remove(_at);
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
