(function (root) {
  const CrystalCove = {};

  CrystalCove.formatStars = count => {
    if (typeof count !== 'number' || count < 1) return '';
    if (count < 1000) return String(count);
    return `${(count / 1000).toFixed(1).replace(/\.0$/, '')}k`;
  };

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
