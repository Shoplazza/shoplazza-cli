export async function shouldRedirect() {
  const response = await fetch('/should-redirect');
  const data = await response.json();
  return data.shouldRedirect;
}
