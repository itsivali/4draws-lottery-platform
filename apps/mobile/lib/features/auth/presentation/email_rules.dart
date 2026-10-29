/// A light check before sending; the API normalises and validates the address itself.
bool isLikelyEmail(String input) => RegExp(r'^[^@\s]+@[^@\s]+\.[^@\s]+$').hasMatch(input.trim());

String? emailFieldError(String? v) =>
    isLikelyEmail(v ?? '') ? null : 'Enter an email address such as name@example.com.';
