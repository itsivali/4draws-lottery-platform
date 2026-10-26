import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';

/// The outcome of checking a revealed spin on this device.
sealed class SpinCheck {
  const SpinCheck();
}

/// The seed matches the commitment shown before the spin, and recomputing gives [slot].
final class SpinVerified extends SpinCheck {
  const SpinVerified(this.slot);
  final int slot;
}

/// The revealed seed is not the one committed to before the spin.
final class CommitmentMismatch extends SpinCheck {
  const CommitmentMismatch();
}

final class InvalidSpinInput extends SpinCheck {
  const InvalidSpinInput(this.reason);
  final String reason;
}

final _clientSeed = RegExp(r'^[A-Za-z0-9_-]{1,64}$');
const _maxSlots = 1 << 20;

// 2^32 as a literal: on the web `1 << 32` wraps to 1 (JavaScript bit operations are 32-bit); this value is exact there too.
const _twoTo32 = 4294967296;

/// Recomputes a spin from its revealed server seed, independently of the server, following the published
/// specification (api/openapi.yaml, `GET /v1/fairness`; internal/game/fair):
///
///     for round = 0, 1, 2, …:
///         mac = HMAC-SHA256(key = seed bytes, message = "<client seed>:<nonce>:<round>")
///         for each big-endian 32-bit word w of mac: if w < 2^32 − (2^32 mod slots) → slot = w mod slots
///
/// This only checks a past spin; nothing in the app ever derives an outcome.
SpinCheck verifySpin({
  required String revealedSeedHex,
  required String commitment,
  required String clientSeed,
  required int nonce,
  required int slots,
}) {
  final seed = _hex(revealedSeedHex);
  if (seed == null || seed.length != 32) return const InvalidSpinInput('the server seed must be 64 hex characters');
  if (!_clientSeed.hasMatch(clientSeed)) return const InvalidSpinInput('invalid client seed');
  if (slots < 2 || slots > _maxSlots) return const InvalidSpinInput('slot count out of range');
  if (nonce < 0) return const InvalidSpinInput('nonce must not be negative');
  if (sha256.convert(seed).toString() != commitment.toLowerCase()) return const CommitmentMismatch();

  final limit = _twoTo32 - _twoTo32 % slots;
  final mac = Hmac(sha256, seed);
  for (var round = 0; ; round++) {
    final sum = Uint8List.fromList(mac.convert(utf8.encode('$clientSeed:$nonce:$round')).bytes);
    final words = ByteData.sublistView(sum);
    for (var i = 0; i < sum.length; i += 4) {
      final w = words.getUint32(i);
      if (w < limit) return SpinVerified(w % slots);
    }
  }
}

List<int>? _hex(String s) {
  if (s.length.isOdd || !RegExp(r'^[0-9a-fA-F]*$').hasMatch(s)) return null;
  return [for (var i = 0; i < s.length; i += 2) int.parse(s.substring(i, i + 2), radix: 16)];
}
