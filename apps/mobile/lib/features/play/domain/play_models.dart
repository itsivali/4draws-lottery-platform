import '../../../core/api/json.dart';

/// One provably fair spin, as the server recorded it.
class Spin {
  const Spin({
    required this.chainId,
    required this.commitment,
    required this.clientSeed,
    required this.nonce,
    required this.slot,
    required this.lucky,
    required this.slots,
    required this.luckySlots,
  });

  factory Spin.fromJson(JsonObject j) => Spin(
    chainId: j.str('chain_id'),
    commitment: j.str('commitment'),
    clientSeed: j.str('client_seed'),
    nonce: j.integer('nonce'),
    slot: j.integer('slot'),
    lucky: j.boolean('lucky'),
    slots: j.integer('slots'),
    luckySlots: j.integer('lucky_slots'),
  );

  final String chainId;
  final String commitment;
  final String clientSeed;
  final int nonce;

  /// 0 to slots-1; slots below [luckySlots] are lucky.
  final int slot;
  final bool lucky;
  final int slots;
  final int luckySlots;
}

/// A paid play: its ticket and its spin.
class Play {
  const Play({
    required this.ticketId,
    required this.playDate,
    required this.seq,
    required this.priceKes,
    required this.status,
    required this.createdAt,
    required this.spin,
  });

  factory Play.fromJson(JsonObject j) => Play(
    ticketId: j.str('ticket_id'),
    playDate: j.date('play_date'),
    seq: j.integer('seq'),
    priceKes: j.integer('price_kes'),
    status: j.str('status'),
    createdAt: j.instant('created_at'),
    spin: Spin.fromJson(j.object('spin')),
  );

  final String ticketId;

  /// The Nairobi calendar day the play counts for.
  final DateTime playDate;

  /// 1 to 4: which play of the day this was.
  final int seq;
  final int priceKes;

  /// `paid` or `refunded`.
  final String status;
  final DateTime createdAt;
  final Spin spin;
}

/// The player's play day so far, as the server counts it.
class Today {
  const Today({
    required this.playDate,
    required this.plays,
    required this.playsLeft,
    required this.lucky,
    required this.chances,
    required this.priceKes,
    required this.tickets,
  });

  factory Today.fromJson(JsonObject j) => Today(
    playDate: j.date('play_date'),
    plays: j.integer('plays'),
    playsLeft: j.integer('plays_left'),
    lucky: j.boolean('lucky'),
    chances: j.integer('chances'),
    priceKes: j.integer('price_kes'),
    tickets: j.list('tickets', Play.fromJson),
  );

  final DateTime playDate;
  final int plays;
  final int playsLeft;

  /// A spin today landed on a lucky slot, which doubles today's draw chances.
  final bool lucky;

  /// Draw chances today's plays earned (from the tier table, doubled on a lucky day).
  final int chances;
  final int priceKes;
  final List<Play> tickets;
}

/// The answer to a play: the play, the day after it, and the confirmed balance after the stake.
class PlayResult {
  const PlayResult({required this.play, required this.today, required this.balanceKes, required this.replayed});

  factory PlayResult.fromJson(JsonObject j, {bool replayed = false}) => PlayResult(
    play: Play.fromJson(j.object('play')),
    today: Today.fromJson(j.object('today')),
    balanceKes: j.integer('balance_kes'),
    replayed: replayed,
  );

  final Play play;
  final Today today;
  final int balanceKes;

  /// The server returned a play it had already made with this idempotency key (nothing new was charged).
  final bool replayed;
}
