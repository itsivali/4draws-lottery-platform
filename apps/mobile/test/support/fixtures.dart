// JSON shaped exactly like the Go handlers write it (internal/api/*.go), for tests and the fake API.

Map<String, Object?> spinJson({int slot = 6, bool lucky = false, int nonce = 0, int slots = 12, int luckySlots = 1}) =>
    {
      'chain_id': 'chain-1',
      'commitment': 'c' * 64,
      'client_seed': 'player-seed',
      'nonce': nonce,
      'slot': slot,
      'lucky': lucky,
      'slots': slots,
      'lucky_slots': luckySlots,
    };

Map<String, Object?> playJson({String ticket = 'ticket-1', int seq = 1, int slot = 6, bool lucky = false}) => {
  'ticket_id': ticket,
  'play_date': '2026-10-26',
  'seq': seq,
  'price_kes': 100,
  'status': 'paid',
  'created_at': '2026-10-26T10:00:00+03:00',
  'spin': spinJson(slot: slot, lucky: lucky, nonce: seq - 1),
};

Map<String, Object?> todayJson({
  int plays = 0,
  int chances = 0,
  bool lucky = false,
  List<Map<String, Object?>>? tickets,
}) => {
  'play_date': '2026-10-26',
  'plays': plays,
  'plays_left': 4 - plays,
  'lucky': lucky,
  'chances': chances,
  'price_kes': 100,
  'tickets': tickets ?? <Object>[],
};

Map<String, Object?> playResultJson({
  String ticket = 'ticket-1',
  int slot = 6,
  bool lucky = false,
  int seq = 1,
  int chances = 1,
  int balance = 850,
}) {
  final play = playJson(ticket: ticket, seq: seq, slot: slot, lucky: lucky);
  return {
    'play': play,
    'today': todayJson(plays: seq, chances: chances, lucky: lucky, tickets: [play]),
    'balance_kes': balance,
  };
}

Map<String, Object?> drawJson({
  String id = 'draw-1',
  String type = 'daily',
  String status = 'open',
  List<Map<String, Object?>> results = const [],
}) {
  final frozen = status != 'open';
  final conducted = ['conducted', 'approved', 'paid'].contains(status);
  return {
    'id': id,
    'draw_type': type,
    'period_start': '2026-10-26',
    'period_end': '2026-10-26',
    'status': status,
    'freeze_at': '2026-10-27T00:05:00+03:00',
    'entries': frozen ? 42 : 0,
    'entry_list_hash': frozen ? 'e' * 64 : null,
    'seed_commit': frozen ? 'f' * 64 : null,
    'drand_round': frozen ? 123456 : null,
    'drand_round_at': frozen ? '2026-10-27T09:00:00+03:00' : null,
    'revealed_seed': conducted ? 'a' * 64 : null,
    'beacon': conducted ? {'round': 123456, 'randomness': 'b' * 64, 'signature': 'c' * 96} : null,
    'certificate_hash': conducted ? 'd' * 64 : null,
    'voided_reason': null,
    'rerun_of': null,
    'signoffs': <Object>[],
    'results': results,
  };
}

Map<String, Object?> walletJson({int balance = 850, int debt = 0}) => {
  'balance_kes': balance,
  'debt_kes': debt,
  'transactions': [
    {
      'id': 'tx-3',
      'kind': 'stake',
      'amount_kes': -100,
      'reference_type': 'ticket',
      'reference_id': 'ticket-1',
      'created_at': '2026-10-26T10:00:00+03:00',
    },
    {
      'id': 'tx-2',
      'kind': 'deposit_excise',
      'amount_kes': -50,
      'reference_type': 'payment',
      'reference_id': 'pay-1',
      'created_at': '2026-10-26T09:00:00+03:00',
    },
    {
      'id': 'tx-1',
      'kind': 'deposit',
      'amount_kes': 1000,
      'reference_type': 'payment',
      'reference_id': 'pay-1',
      'created_at': '2026-10-26T09:00:00+03:00',
    },
  ],
  'next_before': null,
};

Map<String, Object?> topUpJson({String id = 'pay-1', String status = 'pending', int amount = 500, int? confirmed}) => {
  'id': id,
  'status': status,
  'amount_requested_kes': amount,
  'amount_confirmed_kes': confirmed,
  'created_at': '2026-10-26T09:00:00+03:00',
};

Map<String, Object?> payoutJson({String id = 'po-1', String status = 'processing', int amount = 400}) => {
  'id': id,
  'kind': 'withdrawal',
  'status': status,
  'amount_kes': amount,
  'tax_kes': 0,
  'debt_repaid_kes': 0,
  'net_kes': amount,
  'receipt': status == 'paid' ? 'SGH000001' : null,
  'created_at': '2026-10-26T11:00:00+03:00',
};

Map<String, Object?> accountJson({
  String kyc = 'verified',
  bool phoneVerified = true,
  String status = 'active',
  Map<String, Object?>? restriction,
  String? closureRequestedAt,
}) => {
  'user_id': 'user-1',
  'msisdn': '254712345678',
  'phone_verified': phoneVerified,
  'kyc_status': kyc,
  'status': status,
  'created_at': '2026-10-01T09:00:00+03:00',
  'restriction': restriction,
  'closure_requested_at': closureRequestedAt,
};

Map<String, Object?> problemJson(int status, String slug, String title, [String? detail]) => {
  'type': 'https://lottery.example/problems/$slug',
  'title': title,
  'status': status,
  'detail': ?detail,
};
