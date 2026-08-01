export default function EntityDetailLoading() {
  return (
    <main className="mx-auto flex w-full max-w-3xl animate-pulse flex-col gap-6 px-4 py-8">
      <header className="border-b border-border pb-5">
        <div className="h-5 w-44 rounded-full bg-muted" />
        <div className="mt-4 h-9 w-72 max-w-full rounded-xl bg-muted" />
        <div className="mt-3 h-4 w-full max-w-xl rounded bg-muted/70" />
      </header>
      {[0, 1, 2].map((item) => (
        <div key={item} className="h-44 rounded-xl border bg-muted/30" />
      ))}
    </main>
  );
}
