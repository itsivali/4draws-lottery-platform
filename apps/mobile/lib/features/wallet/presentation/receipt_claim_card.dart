import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme/tokens.dart';
import '../../../core/api/api_failure.dart';
import '../../../core/api/idempotency.dart';
import '../../../core/widgets/busy_button.dart';
import '../../../core/widgets/failure_text.dart';
import '../../../core/widgets/inline_banner.dart';
import '../../../core/widgets/status_pill.dart';
import '../application/wallet_providers.dart';
import '../domain/wallet_models.dart';

/// The receipt from the player's M-Pesa message, for a top-up that settled without being credited (owner,
/// 2026-10-31). The server asks M-Pesa about it when it can and credits the top-up once M-Pesa confirms it;
/// otherwise the team confirms it from the statement. The card says which of those is happening.
class ReceiptClaimCard extends ConsumerStatefulWidget {
  const ReceiptClaimCard({super.key, required this.topUp, required this.onChanged, this.compact = false});

  final TopUp topUp;

  /// Called once the server has taken the receipt (or on "Check again"): the caller re-reads the top-up.
  final VoidCallback onChanged;

  /// For a failed top-up: folded behind a question, since most failed prompts took no money.
  final bool compact;

  @override
  ConsumerState<ReceiptClaimCard> createState() => _ReceiptClaimCardState();
}

class _ReceiptClaimCardState extends ConsumerState<ReceiptClaimCard> {
  final _receipt = TextEditingController();
  final _key = newIdempotencyKey();
  var _open = false;
  var _busy = false;
  String? _fieldError;
  String? _bannerTitle;
  String? _bannerDetail;

  static final _receiptRE = RegExp(r'^[A-Z0-9]{10}$');

  @override
  void dispose() {
    _receipt.dispose();
    super.dispose();
  }

  Future<void> _send() async {
    final code = _receipt.text.trim().toUpperCase();
    if (!_receiptRE.hasMatch(code)) {
      setState(() => _fieldError = 'The receipt is 10 letters and digits, like QJ7XK2M9PL.');
      return;
    }
    setState(() {
      _busy = true;
      _fieldError = null;
      _bannerTitle = null;
    });
    try {
      await ref.read(walletRepositoryProvider).claimReceipt(id: widget.topUp.id, receipt: code, idempotencyKey: _key);
      if (!mounted) return;
      ref.invalidate(topUpsProvider);
      widget.onChanged();
    } on ApiFailure catch (f) {
      if (!mounted) return;
      final d = describeFailure(f);
      setState(() {
        if (f is ProblemFailure && f.slug == 'invalid-receipt') {
          _fieldError = f.detail ?? d.title;
        } else {
          _bannerTitle = d.title;
          _bannerDetail = f is ProblemFailure ? f.detail : d.detail;
        }
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    final claim = widget.topUp.claim;
    if (claim != null && claim.status.open) {
      return _Note(
        tone: StatusTone.pending,
        title: 'Receipt ${claim.receipt} sent',
        body:
            "We're confirming it with M-Pesa. Your wallet is credited as soon as M-Pesa confirms the payment; if it "
            'cannot, our team checks the statement. You will not be charged again.',
        action: TextButton(
          onPressed: () {
            widget.onChanged();
            ref.invalidate(topUpsProvider); // the history shows the same answer
          },
          child: const Text('Check again'),
        ),
      );
    }
    if (claim != null && claim.status == ReceiptClaimStatus.confirmed) {
      return _Note(
        tone: StatusTone.success,
        title: 'Receipt ${claim.receipt} confirmed',
        body: 'M-Pesa confirmed the payment and your wallet is credited.',
      );
    }
    if (widget.compact && !_open) {
      return Align(
        alignment: Alignment.centerLeft,
        child: TextButton(
          onPressed: () => setState(() => _open = true),
          child: const Text('Did money leave your M-Pesa? Enter the receipt'),
        ),
      );
    }
    final earlier = switch (claim?.status) {
      ReceiptClaimStatus.unverified =>
        'M-Pesa could not confirm receipt ${claim!.receipt}${claim.note == null ? '' : ': ${claim.note}'}. Our team '
            'will check the statement. If the code was mistyped, correct it here.',
      ReceiptClaimStatus.rejected =>
        'Receipt ${claim!.receipt} was not accepted${claim.note == null ? '' : ': ${claim.note}'}.',
      _ => null,
    };
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (earlier != null) ...[
          InlineBanner(
            tone: claim!.status == ReceiptClaimStatus.unverified ? StatusTone.warning : StatusTone.neutral,
            title: claim.status == ReceiptClaimStatus.unverified
                ? 'Not confirmed by M-Pesa yet'
                : 'Receipt not accepted',
            message: earlier,
          ),
          const SizedBox(height: Space.md),
        ],
        Text('Got an M-Pesa message for this payment?', style: t.titleSmall),
        const SizedBox(height: Space.xs),
        Text(
          'Enter the receipt code from the message (10 letters and digits). We check it with M-Pesa and credit your '
          'wallet once the payment is confirmed. You will not be charged again.',
          style: t.bodyMedium,
        ),
        const SizedBox(height: Space.md),
        if (_bannerTitle != null) ...[
          InlineBanner(tone: StatusTone.error, title: _bannerTitle!, message: _bannerDetail, live: true),
          const SizedBox(height: Space.md),
        ],
        TextField(
          controller: _receipt,
          enabled: !_busy,
          textCapitalization: TextCapitalization.characters,
          autocorrect: false,
          enableSuggestions: false,
          maxLength: 10,
          inputFormatters: [FilteringTextInputFormatter.allow(RegExp('[A-Za-z0-9]')), _upper],
          decoration: InputDecoration(
            labelText: 'M-Pesa receipt',
            hintText: 'QJ7XK2M9PL',
            counterText: '',
            errorText: _fieldError,
          ),
          onSubmitted: (_) => _send(),
        ),
        const SizedBox(height: Space.md),
        BusyButton(label: 'Send receipt', busy: _busy, onPressed: _send),
      ],
    );
  }
}

final _upper = TextInputFormatter.withFunction(
  (_, v) => TextEditingValue(text: v.text.toUpperCase(), selection: v.selection, composing: v.composing),
);

class _Note extends StatelessWidget {
  const _Note({required this.tone, required this.title, required this.body, this.action});

  final StatusTone tone;
  final String title;
  final String body;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [StatusPill(tone: tone, label: title)],
        ),
        const SizedBox(height: Space.sm),
        Text(body, style: t.bodyMedium),
        if (action != null) Align(alignment: Alignment.centerLeft, child: action),
      ],
    );
  }
}
