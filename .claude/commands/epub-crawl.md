Run the EPUB crawl stress test pipeline: discover new EPUBs, validate with both epubverify and epubcheck, and report discrepancies.

Follow these steps:

1. Build epubverify: `make build`
2. Run the crawler to discover and download new EPUBs:
   ```
   bash scripts/epub-crawler.sh --limit 10
   ```
   Use `--limit 10` for a quick run, or increase for deeper testing.
   Use `--source gutenberg` (or `standardebooks`, `feedbooks`) to target one source.
   Use `--dry-run` to preview what would be downloaded.

3. If epubcheck is available, validate crawled EPUBs:
   ```
   bash scripts/crawl-validate.sh
   ```

4. Generate a discrepancy report:
   ```
   bash scripts/crawl-report.sh
   ```

5. Review the report output. If there are false positives or false negatives:
   - Investigate the specific check IDs mentioned
   - Check the validation JSON in `stress-test/crawl-results/`
   - Consider filing issues with `bash scripts/crawl-report.sh --file-issues`

6. Summarize findings: how many EPUBs were crawled, the agreement rate, and any discrepancies found.
