(function (root) {
  const CrystalCove = {};

  CrystalCove.copyText = async (text, clipboard) => {
    if (!clipboard || typeof clipboard.writeText !== 'function') return 'manual';
    try {
      await clipboard.writeText(text);
      return 'copied';
    } catch {
      return 'manual';
    }
  };

  if (typeof module === 'object' && module.exports) module.exports = CrystalCove;
  else root.CrystalCove = CrystalCove;
})(typeof window === 'undefined' ? globalThis : window);
