import '../../../core/api/json.dart';

/// A seed chain. The active one's server seed is secret (only its [commitment] is known); a rotated one has its
/// [revealedSeed], with which every spin on it can be recomputed.
class Chain {
  const Chain({
    required this.id,
    required this.commitment,
    required this.clientSeed,
    required this.nextNonce,
    required this.revealedSeed,
    required this.createdAt,
    required this.revealedAt,
  });

  factory Chain.fromJson(JsonObject j) => Chain(
    id: j.str('id'),
    commitment: j.str('commitment'),
    clientSeed: j.str('client_seed'),
    nextNonce: j.integer('next_nonce'),
    revealedSeed: j.strOrNull('revealed_seed'),
    createdAt: j.instant('created_at'),
    revealedAt: j.instantOrNull('revealed_at'),
  );

  final String id;
  final String commitment;
  final String clientSeed;
  final int nextNonce;
  final String? revealedSeed;
  final DateTime createdAt;
  final DateTime? revealedAt;
}

class Fairness {
  const Fairness({required this.active, required this.rotated});

  factory Fairness.fromJson(JsonObject j) =>
      Fairness(active: Chain.fromJson(j.object('active')), rotated: j.list('rotated', Chain.fromJson));

  final Chain active;
  final List<Chain> rotated;
}
