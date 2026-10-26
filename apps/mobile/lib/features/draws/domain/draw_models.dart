import '../../../core/api/json.dart';

enum DrawType { daily, weekly, monthly, quarterly }

/// The lifecycle the API publishes. `open` takes entries; `frozen` has numbered and hashed them and committed the
/// seed; `signed` waits for the drand round; `conducted` has drawn winners; `approved` has two officials' approval;
/// `paid` has paid the prizes; `voided` was cancelled and re-run as a new draw.
enum DrawStatus { open, frozen, signed, conducted, approved, paid, voided }

class DrawPrize {
  const DrawPrize({required this.rank, required this.amountKes, required this.entryNo});

  factory DrawPrize.fromJson(JsonObject j) =>
      DrawPrize(rank: j.integer('rank'), amountKes: j.integer('amount_kes'), entryNo: j.integer('entry_no'));

  final int rank;
  final int amountKes;
  final int entryNo;
}

class Beacon {
  const Beacon({required this.round, required this.randomness, required this.signature});

  factory Beacon.fromJson(JsonObject j) =>
      Beacon(round: j.integer('round'), randomness: j.str('randomness'), signature: j.str('signature'));

  final int round;
  final String randomness;
  final String signature;
}

class Signoff {
  const Signoff({required this.step, required this.attestation, required this.signedAt});

  factory Signoff.fromJson(JsonObject j) =>
      Signoff(step: j.str('step'), attestation: j.str('attestation'), signedAt: j.instant('signed_at'));

  final String step;
  final String attestation;
  final DateTime signedAt;
}

class Draw {
  const Draw({
    required this.id,
    required this.type,
    required this.periodStart,
    required this.periodEnd,
    required this.status,
    required this.freezeAt,
    required this.entries,
    this.entryListHash,
    this.seedCommit,
    this.drandRound,
    this.drandRoundAt,
    this.revealedSeed,
    this.beacon,
    this.certificateHash,
    this.voidedReason,
    this.rerunOf,
    this.signoffs = const [],
    this.results = const [],
  });

  factory Draw.fromJson(JsonObject j) => Draw(
    id: j.str('id'),
    type: DrawType.values.byName(j.str('draw_type')),
    periodStart: j.date('period_start'),
    periodEnd: j.date('period_end'),
    status: DrawStatus.values.byName(j.str('status')),
    freezeAt: j.instant('freeze_at'),
    entries: j.integer('entries'),
    entryListHash: j.strOrNull('entry_list_hash'),
    seedCommit: j.strOrNull('seed_commit'),
    drandRound: j.intOrNull('drand_round'),
    drandRoundAt: j.instantOrNull('drand_round_at'),
    revealedSeed: j.strOrNull('revealed_seed'),
    beacon: switch (j.objectOrNull('beacon')) {
      final b? => Beacon.fromJson(b),
      null => null,
    },
    certificateHash: j.strOrNull('certificate_hash'),
    voidedReason: j.strOrNull('voided_reason'),
    rerunOf: j.strOrNull('rerun_of'),
    signoffs: j.list('signoffs', Signoff.fromJson),
    results: j.list('results', DrawPrize.fromJson),
  );

  final String id;
  final DrawType type;
  final DateTime periodStart;
  final DateTime periodEnd;
  final DrawStatus status;
  final DateTime freezeAt;
  final int entries;
  final String? entryListHash;
  final String? seedCommit;
  final int? drandRound;
  final DateTime? drandRoundAt;
  final String? revealedSeed;
  final Beacon? beacon;
  final String? certificateHash;
  final String? voidedReason;
  final String? rerunOf;
  final List<Signoff> signoffs;
  final List<DrawPrize> results;

  /// Winners are drawn (the results are final once two officials approve).
  bool get hasResults => status == DrawStatus.conducted || status == DrawStatus.approved || status == DrawStatus.paid;
}

class MyEntry {
  const MyEntry({required this.entryNo, required this.playDate, required this.ticketId});

  factory MyEntry.fromJson(JsonObject j) =>
      MyEntry(entryNo: j.integer('entry_no'), playDate: j.date('play_date'), ticketId: j.str('ticket_id'));

  final int entryNo;
  final DateTime playDate;
  final String ticketId;
}

class MyDrawEntries {
  const MyDrawEntries({required this.drawId, required this.entries, required this.prizes});

  factory MyDrawEntries.fromJson(JsonObject j) => MyDrawEntries(
    drawId: j.str('draw_id'),
    entries: j.list('entries', MyEntry.fromJson),
    prizes: j.list('prizes', DrawPrize.fromJson),
  );

  final String drawId;
  final List<MyEntry> entries;

  /// Prizes the player's own entries won, as the server reports them.
  final List<DrawPrize> prizes;
}
