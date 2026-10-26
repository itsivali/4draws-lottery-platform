import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/features/fairness/domain/fair_verifier.dart';

void main() {
  // The same vectors the server (internal/game/fair) and tools/verifier must reproduce: one source of truth.
  final vectors = (jsonDecode(
    File('../../internal/game/fair/testdata/vectors.json').readAsStringSync(),
  ) as List<Object?>).cast<Map<String, Object?>>();

  test('reproduces every published test vector, including a rejected first word', () {
    expect(vectors, hasLength(greaterThanOrEqualTo(9)));
    for (final v in vectors) {
      final r = verifySpin(
        revealedSeedHex: v['server_seed']! as String,
        commitment: v['commitment']! as String,
        clientSeed: v['client_seed']! as String,
        nonce: v['nonce']! as int,
        slots: v['slots']! as int,
      );
      expect(r, isA<SpinVerified>(), reason: '$v');
      expect((r as SpinVerified).slot, v['slot'], reason: '$v');
    }
  });

  test('a seed that does not match the commitment shown before the spin is reported, not trusted', () {
    final v = vectors.first;
    final r = verifySpin(
      revealedSeedHex: vectors[1]['server_seed']! as String,
      commitment: v['commitment']! as String,
      clientSeed: v['client_seed']! as String,
      nonce: v['nonce']! as int,
      slots: v['slots']! as int,
    );
    expect(r, isA<CommitmentMismatch>());
  });

  test('malformed inputs are refused', () {
    final v = vectors.first;
    SpinCheck check({String? seed, String? client, int? slots, int? nonce}) => verifySpin(
      revealedSeedHex: seed ?? v['server_seed']! as String,
      commitment: v['commitment']! as String,
      clientSeed: client ?? v['client_seed']! as String,
      nonce: nonce ?? v['nonce']! as int,
      slots: slots ?? v['slots']! as int,
    );
    expect(check(seed: 'zz'), isA<InvalidSpinInput>());
    expect(check(seed: 'ab' * 31), isA<InvalidSpinInput>());
    expect(check(client: 'has space'), isA<InvalidSpinInput>());
    expect(check(slots: 1), isA<InvalidSpinInput>());
    expect(check(nonce: -1), isA<InvalidSpinInput>());
  });
}
