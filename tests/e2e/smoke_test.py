import os
import asyncio
import asyncpg
import sys


async def check_connectivity():
    print("Checking database connectivity...")
    dsn = os.getenv("TEST_DB_DSN")
    if not dsn:
        print("Error: TEST_DB_DSN environment variable not set.")
        sys.exit(1)

    try:
        conn = await asyncpg.connect(dsn)
        print("Successfully connected to the database!")

        # Check if articles table exists
        result = await conn.fetchval(
            """
            SELECT EXISTS (
                SELECT FROM information_schema.tables 
                WHERE table_name = 'articles'
            )
        """
        )
        if result:
            print("Table 'articles' exists.")
        else:
            print(
                "Error: Table 'articles' does not exist. Migrations might not have run."
            )
            sys.exit(1)

        await conn.close()
        print("Smoke test passed successfully.")
    except Exception as e:
        print(f"Connection failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    asyncio.run(check_connectivity())
