/**
 * Browser download helper, shared by every "save this file" path so they behave
 * the same way.
 *
 * The ordering matters and is easy to get subtly wrong: the anchor has to be in
 * the document for some engines to honor the click, and the object URL must NOT
 * be revoked in the same task as the click. Revoking immediately relies on the
 * browser having already snapshotted the blob at navigation time, which is not
 * guaranteed - on engines that resolve the URL asynchronously the download simply
 * never starts, with no error anywhere.
 */
export function triggerDownload(blob: Blob, fileName: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = fileName
  document.body.appendChild(a)
  a.click()
  a.remove()
  // Revoke once the download has had a chance to start, never in this same task.
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
